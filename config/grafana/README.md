# Week-3 Grafana provisioning

`cmd/report -serve -runs-dir runs -listen :9091` exports final report metrics
from the archived experiment data. Configure Prometheus to scrape that process,
then provision `dashboards/week3-owner3.json`.

The dashboard intentionally uses only metric names exported by
`internal/metrics/prometheus.go`. It does not pretend that offline final
experiment aggregation is emitted by the live controller.
