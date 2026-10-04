# RFQ rate alerts

Load `rfq.rules.yaml` through the monitoring stack's Prometheus `rule_files` configuration or as the
`spec` of a Prometheus Operator `PrometheusRule`. This repository ships and tests the rules; it does
not provision them or route notifications. Configure Alertmanager through the existing monitoring
deployment. [Prometheus alert rules](https://prometheus.io/docs/prometheus/latest/configuration/alerting_rules/)
document loading and notification routing.

The supplied policy uses a ten-minute counter rate of at least `0.01` events per second (six
observations per ten minutes) sustained for five minutes. Thresholds, windows, evaluation intervals
and `for` durations are deployment policy in the YAML: tune them for expected order traffic and
deadlines before loading the file. Single expected nonce races and fee-ceiling decisions remain
ordinary Info events in the solver.

| Alert | Signal | First checks |
| --- | --- | --- |
| `RFQNonceConsumedWithoutPeerFills` | Sustained `nonce_consumed` results while this EOA's observed `fill/peer` rate stays zero | Other software using the EOA, backend reconciliation lag, and write-RPC freshness. This signal does not identify the consuming transaction or prove another sender exists. |
| `RFQOrdersExpireAfterNonceRetries` | Sustained evidenced order expirations after exhausting `maxNonceRetries` | Inclusion latency, order/discount deadlines and transaction contention. Exhaustion alone does not fire this alert. |
| `RFQTransactionFeesRepeatedlyCapped` | Sustained initial-send or replacement fee-ceiling decisions | Market fees, per-request profitability ceilings and same-nonce contention. A higher cap is not automatically the right correction. |

The rules calculate rates per process before summing, so pod restarts are handled as counter resets.
They attach the sender's public `address` from `solver_bot_txmanager_account_info` using scrape
labels `namespace,kubernetes_pod` and then aggregate siblings by `namespace,address`. Another EOA's
peer fills cannot suppress an alert, even in the same namespace. Namespace also separates networks
or environments that reuse a sender address. Adapt every recording-rule join and alert match if the
collector uses different pod labels or collects several clusters with overlapping namespaces/pod
names; include the cluster identity throughout those joins and aggregations.

Require exactly one sender identity per scrape pod and the new peer/expiry/fee metrics from the same
binary rollout. Missing identity drops the unattributable signal; missing peer metrics prevent the
nonce-versus-peer comparison. These alert rules do not replace scrape availability or metric-contract
alerts in the monitoring stack. Counters represent local observations, not unique global orders:
several replicas can report the same order or peer fill, and restarts forget their local observation
deduplication. Tune the thresholds accordingly. Passive late-receipt telemetry is a separate follow-up
and is not required by these rules.

Validate edits with a local `promtool` (the committed tests are verified with Prometheus `v3.5.0`):

```sh
make test-alerts
```

Set `PROMTOOL=/path/to/promtool` when it is not on `PATH`. CI runs the same target with a pinned,
checksum-verified official Prometheus `v3.5.0` binary.

The unit cases cover single sibling races/caps, sustained signals and pending time, peer-fill
suppression, EOA and namespace isolation, actual expiry versus retry exhaustion, counter resets,
initial/replacement cap aggregation and missing sender identity.
[Prometheus rule unit tests](https://prometheus.io/docs/prometheus/latest/configuration/unit_testing_rules/)
describe the test format.
