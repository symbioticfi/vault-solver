# RFQ rate alert

Load `rfq.rules.yaml` through Prometheus `rule_files` or the monitoring stack's
`PrometheusRule`. Deployment owns loading and notification routing.

`RFQOrdersExpireAfterNonceRetries` reports sustained evidenced order expirations
after exhausting the integration's retry budget. The supplied threshold is
`0.01` events per second over ten minutes, sustained for five minutes. Tune the
YAML for expected order traffic and deadlines. Check inclusion latency,
order deadlines and transaction contention before changing retry limits.

Rates are calculated per scrape process using `namespace,kubernetes_pod`.
Adapt these labels to the collector and include cluster identity if needed.
Counters are local observations; restarts reset them. The rule uses integration
workflow metrics and does not require SDK transaction telemetry.

Run `make test-alerts` with the pinned Prometheus `v3.15.1` promtool, or set
`PROMTOOL=/path/to/promtool`. Tests cover isolated events, sustained expiration
and process isolation.
