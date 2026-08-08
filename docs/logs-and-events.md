# Logs, events & observability

pmmcp keeps three separate records of what happens, plus the machinery to watch, search, export, and notify on them. Knowing which record answers which question — and reaching for the right one — is most of operating pmmcp well.

The three, from [Concepts](concepts.md#the-three-streams):

- **Logs** — what a process *said* (its stdout/stderr).
- **Events** — what the *system* did to a process (started, exited, became unhealthy, restarted).
- **Audit** — who *asked* for a control-plane action (and who was denied).

Plus **ports/URLs**, **webhooks**, and **metrics**.

---

## Logs

The daemon captures each process's stdout and stderr to files under its state directory — `logs/<proc-id>/stdout.log` and `stderr.log` (see [Configuration → Paths](configuration.md#paths-the-daemon-computes)). Capture is continuous and survives client disconnects and daemon restarts, so the logs are there when you need them, not scrolled off a terminal you closed.

### Reading

```bash
pmmcp logs   web              # tail the last 100 lines (both streams)
pmmcp grep   web "GET /api"   # regex search across captured output
pmmcp errors web              # just the error-looking lines
```

`grep` takes a real regular expression. `errors` uses a heuristic to surface the lines that look like failures — a fast first look when something's wrong.

### Rotation

Logs are rotated so they never fill your disk, controlled in [`daemon.toml`](configuration.md):

```toml
[logs]
max_file_mb = 10     # rotate the active file at 10 MiB
max_files   = 5      # keep 5 rotated files per stream
compress    = true   # gzip the rotated ones
```

Rotation is enforced *live* as the process writes, and rotated files are kept (gzipped) so `grep` and `logs` still reach recent history. Files are written `0600`.

### Streaming and structured logs

For a live feed rather than a snapshot, subscribe: `pm_logs_subscribe` streams new lines to an agent (with `pm_logs_unsubscribe` to stop). If a process emits structured JSON or logfmt lines, pmmcp can parse a severity level from them, so you can filter to `warn` and above. And secret values are [redacted](secrets.md#redaction) on the write path — a connection string a dependency logs is scrubbed before it hits disk.

### Exporting

Bundle a process's (or group's) logs together with its events, status, and a manifest — for a bug report or an archive:

```bash
pmmcp logs export name=web            # writes an archive under <state>/exports
```

The bundle carries a `manifest.json` with checksums, and redaction is applied to its contents. `pmmcp logs ship` sends logs to a configured sink for centralized collection.

---

## Events

Events are the system's narration: time-ordered `evt-…` records of lifecycle facts, distinct from anything the process printed. Types are namespaced — `process.*` (started, exited, crashed, restarted), `health.*`, `group.*`, `dependency.*`, `watch.*`, `declare.*`, `daemon.*`, `session.*`.

```bash
pmmcp events            # the last 50 events across the project
pmmcp events <proc-id>  # events for one process
```

```
2026-08-08T04:12:03Z  evt-01J…  process.crashed  web exited with signal SIGSEGV
2026-08-08T04:12:04Z  evt-01J…  process.restarted  web -> proc-01K… (attempt 1)
```

Events are where you see a restart *happen* — the crash, the backoff, the relaunch — as a sequence, while the log tells you *why* it crashed. An agent can subscribe (`pm_events_subscribe` / `_unsubscribe`, list with `_subscriptions`) to be notified as events occur, instead of polling.

Events are retained on a bound (both age and count) and are separate from the [audit](#audit) stream — a crash is an event; the fact that *nobody stopped it* is what the audit trail shows.

---

## Audit

The audit log answers **"who did what?"** — the accountability trail for every control-plane mutation, and every *denied* attempt. Records are `aud-…` and capture the actor's session and client, the action, the target, the result, and the reason. Starts, stops, shares, sandbox relaxations, secret writes, config reloads — and the `permission_denied` that got refused — all land here.

```bash
pmmcp audit                       # recent audit records
pmmcp audit action=process.stop   # filter by action, actor, outcome, time range
```

This is the record you read after an incident: *did the agent stop the database, or did it fall over?* The event says it stopped; the audit says whether a human, an agent, or nobody asked. Secret values are never stored in audit data (argv is, so keep secrets out of argv — see [Secrets](secrets.md)). Reading audit history needs the `audit:read` capability.

---

## Ports and URLs

pmmcp distinguishes two kinds of endpoint:

- **Declared** — ports you stated in the spec or `pmmcp.yaml` (your *intent*).
- **Discovered** — ports the daemon observed the process actually listening on.

```bash
pmmcp ports web
```

`status` and `ports` return both, plus a usable URL. The primary URL prefers a declared, listening HTTP/UI port; it prefers `127.0.0.1` over a `0.0.0.0` bind for the address it hands you. This is how you get "the dev server is at http://127.0.0.1:3000" without reading the log to find the port. A port `0`/auto request is allocated and reported back. Under a strict sandbox, exposure is loopback-only by default.

---

## Webhooks

Webhooks push events to an external URL — a Slack relay, a paging system, your own service. They are **off until you allowlist targets** and are [SSRF-hardened](security.md#outbound-webhooks-and-ssrf).

First, allowlist in [`daemon.toml`](configuration.md) (empty = disabled entirely):

```toml
[webhook]
allowlist = ["https://hooks.example.com"]
```

Then manage hooks:

```bash
pmmcp webhook create url=https://hooks.example.com/pmmcp events=process.crashed
pmmcp webhook list
pmmcp webhook test <id>      # send a test delivery
```

Deliveries are signed with an HMAC signature header so the receiver can verify authenticity, carry a delivery id (`dlv-…`), and are retried with backoff. A target that fails the allowlist or resolves to a blocked IP is refused and audited — a webhook can never be turned into a request against your loopback or a cloud metadata endpoint. Delivery outcomes surface as events; there's no separate deliveries API to poll.

---

## Metrics

For a point-in-time snapshot of the daemon and its processes:

```bash
pmmcp metrics
```

This returns current counts and gauges — processes by state, restart totals, health status, log line rates, IPC latency — as a snapshot (`pm_metrics_snapshot` over MCP). pmmcp does **not** run a metrics HTTP endpoint by default; it stays off the network unless you opt into a loopback-bound scrape endpoint. It is not a long-term time-series database — for that, scrape the snapshot into your own system.

---

## Which one do I reach for?

| Question | Stream |
|----------|--------|
| Why did it crash? | **Logs** (`errors`, `grep`) |
| *Did* it crash — and did it restart? | **Events** |
| Who stopped it / relaxed its sandbox? | **Audit** |
| What port is it on? | **Ports** |
| Tell me *when* it next crashes | **Webhooks** / event subscribe |
| How many are running right now? | **Metrics** |

---

## See also

- [Supervision](supervision.md) — what generates these events
- [Security](security.md) — webhook SSRF policy, audit guarantees
- [Secrets](secrets.md) — redaction on the log write path
- [Configuration](configuration.md) — rotation and the webhook allowlist
