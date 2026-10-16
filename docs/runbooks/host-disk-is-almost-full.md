# Runbook: Host disk is almost full

Alert: `Host disk is almost full`

Severity: `ticket`

## Meaning

The local cluster is close to running out of disk, and then Prometheus, Loki, Tempo, Redpanda, and the databases fail to write. The alert fires for each node-local volume that reports more than 90% of its capacity in use, and the condition must stay true for 10 minutes. Each volume reports the filesystem of the k3d node, so the alert fires once for each volume when the host disk is almost full. The `persistentvolumeclaim` label names the volume. The [`deploy/overlays/local/observability/alerting/rules.json`](../../deploy/overlays/local/observability/alerting/rules.json) file holds the query. An email with the `DatasourceError` alert name and this runbook means that Grafana cannot query Prometheus.

## First checks

1. Open Grafana with `kubectl -n flowspace-local port-forward svc/grafana 3000:80`, or use the Grafana port forward of Tilt. Then open the Host filesystem use and Prometheus database size panels of the [Telemetry stack](../../deploy/overlays/local/observability/dashboards/telemetry-stack.json) dashboard at `http://localhost:3000/d/flowspace-telemetry-stack`.
2. In Grafana Explore, run this query against the Prometheus data source to read the disk use that each volume reports:

   ```promql
   max by (persistentvolumeclaim) (kubelet_volume_stats_used_bytes{job="kubelet"} / kubelet_volume_stats_capacity_bytes{job="kubelet"})
   ```

   The same value for each volume means that the host disk is full. Then the next checks find what fills it.
3. On the host, run this command to read the size of each volume in the k3d node `k3d-flowspace-server-0`:

   ```bash
   docker exec k3d-flowspace-server-0 sh -c 'df -h /var/lib/rancher/k3s; du -sh /var/lib/rancher/k3s/storage/*'
   ```

   Each directory name ends with the namespace and the claim name. A volume that holds most of the used space points to that component.
4. On the host, run this command to read the disk use of Docker:

   ```bash
   docker system df
   ```

   A large reclaimable size of images or build cache points to the images that Tilt builds.

## Resolution

### Docker images and build cache fill the disk

1. Run `docker image prune -a` to remove the images that no container uses. Tilt builds the images again on its next run.
2. Run `docker builder prune` to remove the build cache. The next image builds take longer.

The query of the second check returns a value below 0.9 for each volume, and the alert sends a resolved email within 15 minutes.

### A volume of the telemetry stack grows

1. If the volume is `prometheus-server`, compare its size with the `retentionSize` of 4GB in [`prometheus-values.yaml`](../../deploy/overlays/local/observability/prometheus-values.yaml).
2. If the volume is `storage-loki-0` or `storage-tempo-0`, compare its size with the retention in [`loki-values.yaml`](../../deploy/overlays/local/observability/loki-values.yaml) or [`tempo-values.yaml`](../../deploy/overlays/local/observability/tempo-values.yaml), and read the logs of the pod for compaction errors.
3. If the disk stays full and the volume is `storage-loki-0` or `storage-tempo-0`, delete the volume. This step deletes the stored logs or traces, which are disposable under [ADR-0022](../adr/0022-single-host-storage-holds-disposable-data.md). Scale the `loki` or `tempo` StatefulSet to 0, run `kubectl -n flowspace-local delete pvc <claim name>`, and scale it back to 1 replica so that the StatefulSet creates an empty volume.

The size of the volume in the third check drops, and the query of the second check returns a value below 0.9.

### The Docker Desktop disk is too small

1. Open the Resources settings of Docker Desktop, and increase the disk usage limit.

The `Size` value of the `df` output in the third check grows.

## Escalation

If the host disk stays above 90% after these steps, open a GitHub Issue with the `type:fix` label. Attach the result of the second check and the output of the third and fourth checks.
