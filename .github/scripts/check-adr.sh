#!/usr/bin/env bash
# Enforces the ADR-0001 rules that a person cannot be relied on to check.
set -euo pipefail
cd "$(dirname "$0")/../.."

dir=docs/adr
fail=0
err() { echo "::error::$*"; fail=1; }

shopt -s nullglob
files=("$dir"/[0-9][0-9][0-9][0-9]-*.md)
(( ${#files[@]} )) || { echo "::error::no ADRs found under $dir"; exit 1; }

# Numbers are assigned sequentially and never reused.
i=1
while read -r n; do
  printf -v want '%04d' "$i"
  [[ $n == "$want" ]] || err "numbering: expected $want, found $n (duplicate or gap)"
  (( i++ ))
done < <(printf '%s\n' "${files[@]}" | xargs -n1 basename | cut -c1-4 | sort)

for f in "${files[@]}"; do
  # A filename slug carries no article.
  if [[ $(basename "$f" .md) =~ -(the|a|an)- ]]; then
    err "$f: slug contains an article"
  fi
  # An ADR with nothing under Negative is unfinished, not clean.
  if ! awk '/^### Negative/{s=1;next} /^#/{s=0} s&&/^- /{n++} END{exit !(n>0)}' "$f"; then
    err "$f: the Negative section has no entries"
  fi
done

# An ADR cites another by number only if that file already exists.
while read -r ref; do
  compgen -G "$dir/${ref#ADR-}-*.md" >/dev/null \
    || err "$ref is referenced but no such ADR exists"
done < <(grep -oh 'ADR-[0-9]\{4\}' "${files[@]}" | sort -u)

(( fail )) && exit 1
echo "ok: ${#files[@]} ADRs"
