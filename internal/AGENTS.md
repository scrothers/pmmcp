# agents: internal (index)

**Hierarchy:** root [`../AGENTS.md`](../AGENTS.md) is global law → this index → `internal/<pkg>/AGENTS.md` (package specialization). Package briefs never override root security, architecture, parity, or the **≥80% coverage floor**.

Before editing package **P**, read `internal/P/AGENTS.md`. Every package below has that brief plus a human `README.md`.

## Import direction

Leaf → application. Drivers only through `*/drivers` selectors. No `func init()`. Constructor wiring in `daemon` / `main` / selectors.

## Coverage

Per-package statement coverage floor **≥80%** under `./internal/...` (generated `api/gen` excluded). Floor only — do not grind to 100%. See root AGENTS.

## Packages

| Path | Role (one line) |
|------|-----------------|
| api | IPC method names + JSON DTOs + `AllMethods` |
| api/gen/pmmcp/v1 | **Generated** gRPC stubs — do not edit |
| audit | In-memory audit ring buffer |
| authz | Roles, capabilities, share book |
| cli | CLI + MCP tool catalog / go-sdk adapter |
| config | TOML load + platform defaults |
| daemon | pmmcpd control plane |
| daemoncmd | Thin cobra/root entry for `pmmcpd` binary |
| declare | pmmcp.yaml parse/validate/diff/import |
| doctor | Daemon reachability checks |
| domain | Pure process/status/error values |
| engine | Container engine interface |
| engine/docker | Docker CLI engine |
| engine/drivers | Engine selector |
| engine/fake | Test double engine |
| engine/podman | Podman CLI engine |
| event | Domain event bus |
| group | Process groups + depends_on DAG |
| id | Prefixed ULIDs |
| ipc | gRPC Dial/Listen + peer UID filter |
| logcap | Log capture, rotate, redact, tail |
| mcp | MCP resources/prompts helpers |
| observability | Metrics snapshot |
| ports | Listening port discovery (Linux) |
| process | Manager interface + Router |
| process/container | Container-backed Manager |
| process/drivers | Process backend selector |
| process/fake | Test double Manager |
| process/local | OS process Manager + sandbox wrap |
| profile | In-memory profile store |
| project | Project root detect + key |
| prompts | Embedded agent prompt + doc markdown |
| sandbox | Profiles + Policy |
| sandbox/darwin | seatbelt Apply |
| sandbox/linux | Landlock + bwrap |
| sandbox/windows | Job-object Apply |
| secret | Env files, SOPS, secret://, keyring |
| service | Platform install dispatcher |
| service/darwin | launchd plist write |
| service/linux | systemd --user unit write |
| service/windows | Windows service notes/artifacts |
| session | Harness session registry |
| store | ProcessStore interface |
| store/sqlite | modernc SQLite impl |
| supervise | Restart policy / relaunch / health probe |
| version | Build version string |
| watch | Hot-reload path watcher |
| webhook | SSRF-safe webhook registry |
