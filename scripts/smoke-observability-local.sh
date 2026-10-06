#!/bin/sh
set -eu
umask 077

# The script sends a signup, an email verification, and a workspace read with
# known request IDs and trace contexts. It makes sure that Loki finds the log
# lines by request ID. It makes sure that Tempo holds the signup and session
# check traces with the right parents. It makes sure that Grafana links a log
# line to its trace. It makes sure that no stored log line, span, or Loki label
# holds a known secret value.
context=k3d-flowspace
namespace=flowspace-local
identity_port=18082
workspace_port=18086
mail_port=18025
loki_port=13100
tempo_port=13200
grafana_port=13000
response=$(mktemp)
store=$(mktemp)
label_values=$(mktemp)

cleanup() {
  for pid in ${forwards:-}; do
    kill "$pid" 2>/dev/null || true
  done
  wait 2>/dev/null || true
  rm -f "$response" "$store" "$label_values"
}
trap cleanup EXIT HUP INT TERM

fail() {
  printf '%s\n' "$*" >&2
  exit 1
}

for workload in deployment/identity-api deployment/identity-worker deployment/workspace-api deployment/alloy \
  deployment/grafana deployment/mailpit statefulset/loki statefulset/tempo; do
  kubectl --context "$context" -n "$namespace" rollout status "$workload" --timeout=180s >/dev/null
done

# forward <Service> <local port> <Service port> fails when another process
# already listens on the local port, so that no request goes to it.
forwards=
forward() {
  if nc -z 127.0.0.1 "$2" 2>/dev/null; then
    fail "Another process listens on port $2."
  fi
  kubectl --context "$context" -n "$namespace" port-forward --address 127.0.0.1 "svc/$1" "$2:$3" >/dev/null 2>&1 &
  forwards="$forwards $!"
}
forward identity-api "$identity_port" 8080
forward workspace-api "$workspace_port" 8080
forward mailpit-http "$mail_port" 8025
forward loki "$loki_port" 3100
forward tempo "$tempo_port" 3200
forward grafana "$grafana_port" 80
for port in "$identity_port" "$workspace_port" "$mail_port" "$loki_port" "$tempo_port" "$grafana_port"; do
  for attempt in $(seq 1 20); do
    nc -z 127.0.0.1 "$port" 2>/dev/null && break
    sleep 1
  done
  nc -z 127.0.0.1 "$port" 2>/dev/null || fail "The port-forward to port $port did not start."
done
for pid in $forwards; do
  kill -0 "$pid" 2>/dev/null || fail "A port-forward stopped."
done

# The known values use letters only, so that the random digits of a
# timestamp or an ID cannot match them by chance.
letters() { openssl rand -hex "$1" | tr 0-9 g-p; }
run=$(letters 6)
started=$(date +%s)
email="obs-$run@example.test"
password=$(letters 16)
signup_id="obs-signup-$run"
verify_id="obs-verify-$run"
create_id="obs-create-$run"
read_id="obs-read-$run"
parent_span=00f067aa0ba902b7
signup_trace=$(openssl rand -hex 16)
verify_trace=$(openssl rand -hex 16)
create_trace=$(openssl rand -hex 16)
read_trace=$(openssl rand -hex 16)

request() {
  # request <method> <url> <request ID> <trace ID or -> <body or -> [header...]
  method=$1 url=$2 id=$3 trace=$4 body=$5
  shift 5
  set -- --header "X-Request-ID: $id" "$@"
  if [ "$trace" != - ]; then
    set -- --header "traceparent: 00-$trace-$parent_span-01" "$@"
  fi
  if [ "$body" != - ]; then
    set -- --header 'Content-Type: application/json' --data-binary "$body" "$@"
  fi
  curl --silent --show-error --max-time 15 --request "$method" --output "$response" --write-out '%{http_code}' "$@" "$url"
}

status=$(request POST "http://127.0.0.1:$identity_port/v1/accounts" "$signup_id" "$signup_trace" \
  "$(jq -nc --arg email "$email" --arg password "$password" '{email:$email,password:$password}')")
[ "$status" = 200 ] || fail "Signup returned HTTP $status."
access_token=$(jq -er .accessToken "$response")
refresh_token=$(jq -er .refreshToken "$response")

code=
for attempt in $(seq 1 60); do
  message=$(curl --silent --max-time 5 "http://127.0.0.1:$mail_port/api/v1/messages" |
    jq -r --arg email "$email" 'first(.messages[] | select(.To[].Address == $email) | .ID) // empty')
  if [ -n "$message" ]; then
    code=$(curl --silent --max-time 5 "http://127.0.0.1:$mail_port/api/v1/message/$message" |
      jq -r '.Text' | grep -Eo '\b[0-9]{6}\b' | head -n 1)
    break
  fi
  sleep 1
done
[ -n "$code" ] || fail "The verification email did not arrive."

status=$(request POST "http://127.0.0.1:$identity_port/v1/email-verifications" "$verify_id" "$verify_trace" \
  "$(jq -nc --arg code "$code" '{code:$code}')" --header "Authorization: Bearer $access_token")
[ "$status" = 200 ] || fail "Email verification returned HTTP $status."
status=$(request POST "http://127.0.0.1:$workspace_port/v1/workspaces" "$create_id" "$create_trace" '{"name":"Observability smoke"}' \
  --header "Authorization: Bearer $access_token" --header "Idempotency-Key: $run")
[ "$status" = 200 ] || fail "Workspace creation returned HTTP $status."
workspace=$(jq -er .workspace.id "$response")
status=$(request GET "http://127.0.0.1:$workspace_port/v1/workspaces/$workspace" "$read_id" "$read_trace" - \
  --header "Authorization: Bearer $access_token")
[ "$status" = 200 ] || fail "Workspace read returned HTTP $status."

# loki_lines <LogQL> writes each matching log line as one JSON object.
loki_lines() {
  curl --silent --show-error --max-time 30 --get "http://127.0.0.1:$loki_port/loki/api/v1/query_range" \
    --data-urlencode "query=$1" --data-urlencode "start=${started}000000000" --data-urlencode limit=5000 |
    jq -c '.data.result[].values[][1] | fromjson? // empty'
}

# wait_for_line <request ID> <message> writes the first line of the request
# with that message.
wait_for_line() {
  for attempt in $(seq 1 60); do
    line=$(loki_lines "{namespace=\"$namespace\"} | json | request_id=\"$1\"" | jq -c --arg msg "$2" 'select(.msg == $msg)' | head -n 1)
    if [ -n "$line" ]; then
      printf '%s\n' "$line"
      return 0
    fi
    sleep 2
  done
  fail "Loki has no $2 line for request $1."
}

check_line() {
  # check_line <line> <service> <request ID> <trace ID>
  printf '%s\n' "$1" | jq -e --arg service "$2" --arg id "$3" --arg trace "$4" \
    '.service == $service and .environment == "local" and .request_id == $id and .trace_id == $trace' >/dev/null ||
    fail "The $(printf '%s' "$1" | jq -r .msg) line of request $3 does not have service $2, environment local, and trace ID $4."
}

signup_line=$(wait_for_line "$signup_id" identity_request)
check_line "$signup_line" identity-api "$signup_id" "$signup_trace"
check_line "$(wait_for_line "$verify_id" identity_request)" identity-api "$verify_id" "$verify_trace"
check_line "$(wait_for_line "$read_id" request_completed)" workspace-api "$read_id" "$read_trace"
check_line "$(wait_for_line "$read_id" identity_session_check)" identity-api "$read_id" "$read_trace"
printf 'Criterion 1: Loki finds the Identity and Workspace lines by request ID with service, environment, request_id, and trace_id.\n'

# spans <trace ID> writes the spans of the trace as JSON objects with hex IDs.
base64_hex() { printf '%s' "$1" | base64 -d 2>/dev/null | xxd -p | tr -d '\n'; }
spans() {
  curl --silent --max-time 15 "http://127.0.0.1:$tempo_port/api/traces/$1" |
    jq -c '.batches[]? | (.resource.attributes[] | select(.key == "service.name") | .value.stringValue) as $service |
      .scopeSpans[].spans[] | {service: $service, name, kind, spanId, parentSpanId: (.parentSpanId // "")}' |
    while read -r span; do
      printf '%s\n' "$span" | jq -c --arg id "$(base64_hex "$(printf '%s' "$span" | jq -r .spanId)")" \
        --arg parent "$(base64_hex "$(printf '%s' "$span" | jq -r .parentSpanId)")" '.spanId = $id | .parentSpanId = $parent'
    done
}

# wait_for_spans <trace ID> <count> waits until Tempo has at least count spans
# of the trace, and writes them.
wait_for_spans() {
  for attempt in $(seq 1 60); do
    found=$(spans "$1")
    if [ "$(printf '%s' "$found" | grep -c . || true)" -ge "$2" ]; then
      printf '%s\n' "$found"
      return 0
    fi
    sleep 2
  done
  fail "Tempo does not have $2 spans of trace $1."
}

# span_id <spans> <service> <name> <kind> <parent span ID> writes the ID of
# the one matching span.
span_id() {
  id=$(printf '%s\n' "$1" | jq -r --arg service "$2" --arg name "$3" --arg kind "SPAN_KIND_$4" --arg parent "$5" \
    'select(.service == $service and .name == $name and .kind == $kind and .parentSpanId == $parent) | .spanId')
  [ "$(printf '%s' "$id" | grep -c .)" -eq 1 ] || fail "The trace has no single $4 span $3 of $2 under span $5."
  printf '%s' "$id"
}

signup_spans=$(wait_for_spans "$signup_trace" 3)
signup_server=$(span_id "$signup_spans" identity-api "POST /v1/accounts" SERVER "$parent_span")
printf 'Criterion 5: the signup server span is a child of the client traceparent.\n'
regex=$(curl --silent --show-error --max-time 15 \
  --user "admin:$(kubectl --context "$context" -n "$namespace" get secret grafana-admin -o jsonpath='{.data.admin-password}' | base64 -d)" \
  "http://127.0.0.1:$grafana_port/api/datasources/uid/loki" |
  jq -er '.jsonData.derivedFields[] | select(.datasourceUid == "tempo") | .matcherRegex')
linked=$(printf '%s' "$signup_line" | grep -Eo "$regex" | grep -Eo '[0-9a-f]{32}')
[ "$linked" = "$signup_trace" ] || fail "The Grafana trace link of the signup line points to $linked."
printf 'Criterion 2: Tempo has the signup server span, and Grafana links the log line to trace %s.\n' "$signup_trace"

publish=$(span_id "$signup_spans" identity-worker identity.outbox_publish PRODUCER "$signup_server")
span_id "$signup_spans" identity-worker identity.email_delivery CONSUMER "$publish" >/dev/null
printf 'Criterion 4: the signup trace holds the request span, identity.outbox_publish, and identity.email_delivery.\n'

check_session=/flowspace.identity.v1.IdentityService/CheckSession
read_spans=$(wait_for_spans "$read_trace" 3)
read_server=$(span_id "$read_spans" workspace-api "GET /v1/workspaces/{workspace_id}" SERVER "$parent_span")
read_client=$(span_id "$read_spans" workspace-api "$check_session" CLIENT "$read_server")
span_id "$read_spans" identity-api "$check_session" SERVER "$read_client" >/dev/null
printf 'Criterion 3: the workspace read trace holds the Workspace server span, the CheckSession client span, and the Identity CheckSession server span.\n'

# Collect each stored string and number as text. The filters leave out only
# the IDs, timestamps, and durations that telemetry adds, because their
# random digits can contain a six-digit code by chance.
loki_text='(fromjson? // .) | if type == "object" then del(.trace_id, .ts, .duration, .record_age_seconds, .oldest_seconds) else . end |
  [.. | scalars | tostring]'
tempo_text='del(.. | .traceId?, .spanId?, .parentSpanId?, .startTimeUnixNano?, .endTimeUnixNano?, .timeUnixNano?, .flags?) |
  [.. | scalars | tostring]'
for leak in '{"msg":"leak","code":"654321"}' '{"msg":"leak","code":654321}'; do
  printf '%s' "$leak" | jq -Rc "$loki_text" | grep -qF 654321 || fail "The Loki text filter drops a code in $leak."
done
printf '%s' '{"batches":[{"attributes":[{"value":{"intValue":"654321"}}]}]}' | jq -c "$tempo_text" | grep -qF 654321 ||
  fail "The Tempo text filter drops a code in a span attribute."
curl --silent --show-error --max-time 30 --get "http://127.0.0.1:$loki_port/loki/api/v1/query_range" \
  --data-urlencode "query={namespace=\"$namespace\"}" --data-urlencode "start=${started}000000000" --data-urlencode limit=5000 |
  jq -c ".data.result[].values[][1] | $loki_text" >"$store"
for trace in "$signup_trace" "$verify_trace" "$create_trace" "$read_trace"; do
  wait_for_spans "$trace" 1 >/dev/null
  curl --silent --max-time 15 "http://127.0.0.1:$tempo_port/api/traces/$trace" | jq -c "$tempo_text" >>"$store"
done
labels=$(curl --silent --show-error --max-time 15 --get "http://127.0.0.1:$loki_port/loki/api/v1/labels" \
  --data-urlencode "start=${started}000000000" | jq -r '.data[]')
for label in $labels; do
  curl --silent --show-error --max-time 15 --get "http://127.0.0.1:$loki_port/loki/api/v1/label/$label/values" \
    --data-urlencode "start=${started}000000000" | jq -c --arg label "$label" '[$label] + .data' >>"$label_values"
done
cat "$label_values" >>"$store"
# The stored text must hold the request IDs, or the search below proves
# nothing.
grep -qF -- "\"$signup_id\"" "$store" && grep -qF -- "\"$read_id\"" "$store" ||
  fail "The stored text does not hold the request IDs of this run."
for secret in "$email" "obs-$run" "$password" "$code" "$access_token" "$refresh_token"; do
  if grep -qF -- "$secret" "$store"; then
    fail "A stored log line, span, or Loki label holds a known secret value."
  fi
done
printf 'Criterion 8: no stored log line, span, or Loki label holds the known token, password, code, or email values.\n'

for label in $labels; do
  case "$label" in
    service | environment | namespace | service_name) ;;
    *) fail "Loki has the unexpected label $label." ;;
  esac
done
if grep -qE '[0-9a-f]{32}|obs-|/' "$label_values"; then
  fail "A Loki label value is a request ID, a trace ID, or a path."
fi
printf 'Criterion 9: Loki has only the labels %s, and no label value is a request ID, a trace ID, or a path.\n' "$(printf '%s' "$labels" | tr '\n' ' ')"
