# Runbook: Telemetry is dropped

Alert: `Telemetry is dropped`

Severity: `ticket`

## Meaning

Developers lose traces, metrics, or logs, because Alloy refuses or fails to send spans, metric points, or log entries. The dashboards and Explore then show gaps, and other alerts can miss a symptom. The alert fires for each Alloy component that refused or failed to send at least 1 item in the last 5 minutes, and the condition must stay true for 10 minutes. The `component_id` label names the component. The [`deploy/overlays/local/observability/alerting/rules.json`](../../deploy/overlays/local/observability/alerting/rules.json) file holds the query. An email with the `DatasourceError` alert name and this runbook means that Grafana cannot query Prometheus.

## First checks

1. Open Grafana with `kubectl -n flowspace-local port-forward svc/grafana 3000:80`, or use the Grafana port forward of Tilt. Then open the Alloy telemetry loss and Store ingestion failures panels of the [Telemetry stack](../../deploy/overlays/local/observability/dashboards/telemetry-stack.json) dashboard at `http://localhost:3000/d/flowspace-telemetry-stack`. The Alloy telemetry loss panel shows which kind of telemetry Alloy loses.
2. In Grafana Explore, run this query against the Prometheus data source to count the lost items of each Alloy component in the last 10 minutes:

   ```promql
   sum by (component_id) (label_replace(increase(otelcol_receiver_refused_spans_total{job="alloy"}[10m]), "metric", "otelcol_receiver_refused_spans_total", "", "") or label_replace(increase(otelcol_receiver_refused_metric_points_total{job="alloy"}[10m]), "metric", "otelcol_receiver_refused_metric_points_total", "", "") or label_replace(increase(otelcol_exporter_send_failed_spans_total{job="alloy"}[10m]), "metric", "otelcol_exporter_send_failed_spans_total", "", "") or label_replace(increase(prometheus_remote_storage_samples_failed_total{job="alloy"}[10m]), "metric", "prometheus_remote_storage_samples_failed_total", "", "") or label_replace(increase(loki_write_dropped_entries_total{job="alloy"}[10m]), "metric", "loki_write_dropped_entries_total", "", ""))
   ```

   A value above 0 for `otelcol.exporter.otlp.tempo` points to Tempo, for `prometheus.remote_write.prometheus` to Prometheus, and for `loki.write.loki` to Loki. A value above 0 for `otelcol.receiver.otlp.default` means that the memory limiter of Alloy refuses new spans or metric points, usually because a store is down and the export queue fills the memory of Alloy.
3. Inspect the pods of the telemetry stack:

   ```bash
   kubectl -n flowspace-local get pods -l 'app.kubernetes.io/name in (alloy, tempo, loki, prometheus)'
   ```

   No pod, a pod that is not ready, or a high restart count points to that component.
4. Read the export errors of Alloy for the component of the second check, such as `otelcol.exporter.otlp.tempo`:

   ```bash
   kubectl -n flowspace-local logs deployment/alloy --since=15m | grep 'component_id=otelcol.exporter.otlp.tempo'
   ```

   A `connection refused` or `Unavailable` error points to a store that does not run or that Alloy cannot reach. A `rate_limited`, `stream_limited`, or `batch_too_large` reason on `loki_write_dropped_entries_total` points to the limits of Loki.

## Resolution

### A store does not run

1. If Tempo has 0 replicas, run `kubectl -n flowspace-local scale statefulset tempo --replicas=1`. For Loki, scale the `loki` StatefulSet. For Prometheus, run `kubectl -n flowspace-local scale deployment prometheus-server --replicas=1`.
2. If the pod restarts, run `kubectl -n flowspace-local logs <pod> --previous`. Then fix the error.
3. If the error says that the disk is full, follow [`host-disk-is-almost-full.md`](host-disk-is-almost-full.md).

The query of the second check returns 0 for each component, and the alert sends a resolved email within 15 minutes. Alloy does not send the telemetry that it lost again.

### The memory limiter of Alloy refuses telemetry

1. Fix the store that the second check names for an exporter first. The limiter stops refusing when the export queue drains.
2. If no exporter fails, run `kubectl -n flowspace-local top pod -l app.kubernetes.io/name=alloy`. Then compare the memory use with the 320MiB limit of `otelcol.processor.memory_limiter` in [`alloy-values.yaml`](../../deploy/overlays/local/observability/alloy-values.yaml).

The `otelcol.receiver.otlp.default` value of the second check returns to 0.

### Loki rejects log entries

1. Read the `reason` label of `loki_write_dropped_entries_total` in Grafana Explore.
2. Run `kubectl -n flowspace-local logs statefulset/loki | grep -i limit`. Then compare the error with the limits in [`loki-values.yaml`](../../deploy/overlays/local/observability/loki-values.yaml).
3. If the reason is `ingester_error` and the Alloy log shows HTTP 400 errors for old timestamps, check the age of the Alloy pod. After a restart, Alloy reads the pod logs again, and Loki rejects the entries that are too old. This loss happens once and stops within 5 minutes.

The Store ingestion failures panel shows no Loki discarded samples.

## Escalation

If Alloy still loses telemetry after these steps, open a GitHub Issue with the `type:fix` label. Attach the result of the second check, the pod list of the third check, and 3 log lines of the fourth check.
