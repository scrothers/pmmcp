# Supervision & orchestration

Starting a process is the easy part. Keeping it alive, restarting it sanely when it crashes, bringing it back after a reboot, and coordinating several processes that depend on each other — that's supervision, and it's what separates pmmcp from `nohup &`.

This guide covers the process lifecycle, health checks, restart policy, boot relaunch, groups with dependencies, one-shot jobs, and hot reload.

---

## The process lifecycle

Every managed process has a **status** (where it is now) and a **desired state** (where you want it). The daemon continuously reconciles the two.

**Status** is one of:

| Status | Meaning |
|--------|---------|
| `starting` | launched, not yet confirmed up |
| `running` | up (and healthy, if it has a health check) |
| `unhealthy` | up but failing its health check — a *running* sub-state |
| `stopping` | a stop is in progress |
| `exited` | finished on its own (a normal or one-shot exit) |
| `failed` | stopped abnormally / gave up after restarts |
| `crashed` | died unexpectedly (non-zero, signal, OOM) |

The normal path is `starting → running`. From `running` a process may flip to `unhealthy` and back; a deliberate stop goes `stopping → exited`; an unexpected death goes to `crashed` (and, if restarts are exhausted, `failed`). Every transition emits an [event](logs-and-events.md#events).

**Desired state** is `running` or `stopped`. You set it with `enable` / `disable`:

```bash
pmmcp enable  web     # desired = running: relaunched after crash and after a daemon restart
pmmcp disable web     # desired = stopped: stopped now, and stays down
```

A process you just `start` is running but not necessarily *durable*. `enable` makes it durable — part of the set the daemon restores on boot (see [Boot relaunch](#boot-relaunch)).

---

## Stopping: graceful, then forceful

`pmmcp stop` is a **tree kill**, not a single-PID kill:

1. mark the process `stopping`,
2. send `SIGTERM` to the process **and its descendants**,
3. wait a grace period (default 10 seconds) for a clean exit,
4. `SIGKILL` anything still alive,
5. confirm nothing remains and release the process's ports.

This is why a stopped `npm run dev` leaves no orphaned `node` child holding port 3000, and no `nohup.out` behind. On Windows the equivalent is a Job Object terminate. `SIGKILL` is always the last resort, never the first move.

---

## Health checks

A running process isn't necessarily a *ready* one. A health check tells the difference.

**HTTP is the implemented probe.** Give a process a health URL and pmmcp issues an HTTP `GET`; a `2xx`/`3xx` response is healthy. The probe has a `timeout` (default 2 seconds), an `interval`, and a `retries` count (consecutive failures before the process is declared `unhealthy`). A sustained failure moves the process to `unhealthy` and — depending on your restart policy — can trigger a restart.

A process **without** a health URL gets a basic liveness check: is it still running or not. (The health-check type field also names `tcp` and `exec`, but only the HTTP probe is wired today — treat those as reserved.)

Force a probe immediately:

```bash
pmmcp health-check web
```

> **HTTP health checks are loopback-guarded.** By default the probe target must be loopback, and redirects are re-validated — the same [SSRF protection](security.md#outbound-webhooks-and-ssrf) applied to webhooks. This keeps a health check from being turned into a request forgery.

---

## Restart policy

When a process crashes, the daemon can bring it back — with **backoff**, so a process that dies instantly on startup doesn't spin the CPU in a tight restart loop.

You opt a process into auto-restart when you start it (and `enable` makes recovery durable across daemon restarts). The policy has these controls:

- **max retries** — how many times to restart within a window before giving up and marking the process `failed`. The daemon's default cap is 20.
- **backoff** — the delay before the next attempt. By default it grows linearly (base × attempt); set a **multiplier** to make it exponential, a **max backoff** to cap it, and a **jitter** fraction to spread retries out. The daemon's default base is 500 ms.
- The counter resets after the process has run healthy long enough — so an occasional crash weeks apart never accumulates toward the cap.

An **intentional** stop never triggers a restart. Auto-restart is for *unexpected* death, not for your `pmmcp stop`.

Each restart mints a **new `proc-` id**, linked to its predecessor, so "the web server across ten crashes" is a traceable chain of ten records, not one record with a lossy count. Reset the retry budget deliberately when you've fixed the underlying problem:

```bash
pmmcp restart web        # a fresh start; you can clear the restart count as part of this
```

---

## Boot relaunch

When the daemon starts — after a reboot, an upgrade, or a service restart — it reads back the processes whose desired state is `running` and relaunches them. This is on by default (`relaunch.enabled = true` in [`daemon.toml`](configuration.md)).

Only **durable** processes come back: those you `enable`d (or that were `running` with a durable desired state). A one-shot that had already exited stays exited — it isn't re-run. A process you explicitly `disable`d stays down; that explicit intent wins over the relaunch. Before relaunching, the daemon checks whether a persisted PID is still alive, so a daemon restart doesn't double-start something that survived it.

---

## Groups

A **group** runs related processes together with dependency ordering — an API that needs its database up first, a worker that needs the queue. See [Concepts → Project](concepts.md#scoping) for how groups are scoped.

```bash
pmmcp group create name=stack …      # define members and their depends_on edges
pmmcp group start   stack            # start members in dependency order
pmmcp group status  stack            # aggregate phase + per-member readiness
pmmcp group stop    stack            # stop in reverse order
```

- **Ordering** is a DAG. `depends_on` edges determine start order (dependencies first) and stop order (reverse). A dependency cycle is rejected when you define the group, not discovered at start time.
- **Aggregate status** is Kubernetes-shaped: the group reports a `phase` plus a `ready`/`desired` count and per-member status, so `group status` answers "is the whole stack up?" in one look.
- **Members are still individual processes.** Each has its own `proc-` id, logs, and status; you can `status`/`logs`/`restart` one member without touching the group.

Declaratively, a group is a few lines of `pmmcp.yaml` — often the cleanest way to define one. See [Declarative](declarative.md).

---

## One-shot jobs

Not everything is a server. A migration, a build, a data import runs to completion and reports an exit code. Model those as one-shots:

```bash
pmmcp run --name migrate -- ./scripts/migrate.sh
pmmcp wait migrate
```

```
migrate  status=exited  exit_code=0  timed_out=false
```

A one-shot is **never auto-restarted** and is **not** brought back on boot — it ran, it finished, that's the contract. Its exit code is preserved for `wait` to report. Use one-shots for anything where "did it succeed?" matters more than "is it still up?".

---

## Hot reload

A process can watch its own source and restart when files change — the supervised equivalent of a dev server's built-in reloader, but driven by the daemon so it's consistent across languages:

```bash
pmmcp watch set name=web paths=./src action=restart
pmmcp watch status
```

Filesystem events are **debounced** — a burst of saves collapses into a single restart, and the debounce window resets on each new change so a long save storm doesn't thrash. The watcher runs in the daemon, not the client, and honors the process's sandbox: watched paths are still subject to strict-mode rules. Declaratively, watch is a `watch:` block on a service (see [Declarative](declarative.md)).

---

## Container runtime

Any of the above works for a container just as it does for a host process — same `start`/`stop`/`status`/`logs`/health/restart, same ids. Set `runtime: container` and an `image`, and pmmcp runs it through Podman (preferred) or Docker, hardened by default (see [Security → Containers](security.md#containers)). Use it for sidecars — a Postgres or Redis your app depends on — and wire the dependency with a group:

```yaml
# in pmmcp.yaml — api waits for db to be healthy
spec:
  groups:
    - name: stack
      members:
        - name: db
          image: postgres:16
          runtime: container
        - name: api
          argv: ["npm", "start"]
          depends_on: { db: service_healthy }
```

A missing engine is a clear start-time error, never a silent fall-back to the host.

---

## See also

- [Logs, events & observability](logs-and-events.md) — watch a restart happen, see why it crashed
- [Declarative `pmmcp.yaml`](declarative.md) — define groups, health, restart, and watch in one file
- [CLI reference](cli.md) — every lifecycle verb
- [Error reference](reference-errors.md) — `spawn_failed`, `sandbox_failed`, `failed_precondition`
