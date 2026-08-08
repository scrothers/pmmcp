# webhook

Package `webhook` implements an **in-memory registry** of outbound webhook destinations and an **SSRF-safe JSON deliverer**. Webhooks notify external systems (chat ops, CI, custom automations) when interesting events occur; they are **egress only** and never accept inbound control-plane traffic.

Design: outbound webhooks for domain events.

## Concepts

### Hook

| Field | Description |
|-------|-------------|
| `ID` | Unique registry key (caller-assigned; daemon may generate `wh-…`) |
| `URL` | Destination; must pass `ValidateURL` |
| `Events` | Optional event-type filter; empty means “all” (filtering is a higher-layer concern today) |

### Registry

Thread-safe in-memory store:

```go
r:= webhook.NewRegistry
err:= r.Create(webhook.Hook{
 ID: "wh-01…",
 URL: "https://hooks.example.com/pmmcp",
 Events: []string{"process.crashed"},
})
list:= r.List
h, err:= r.Get("wh-01…")
err = r.Delete("wh-01…")
```

- `Create` **upserts** by ID (update path reuses Create with a merged hook).
- Empty ID → `ErrInvalid`.
- URL is validated on every Create (including updates).
- `List` / `Get` return deep-copied `Events` slices so callers cannot mutate the registry through the result.
- Unknown ID on Delete/Get → `ErrNotFound`.

Persistence is **not** provided: the registry lives for the daemon process lifetime unless a higher layer snapshots it.

### Deliverer

```go
d:= &webhook.Deliverer{
 Client: nil, // default: http.Client{Timeout: 10s}
 MaxBody: 0, // default: 64 KiB drain limit
}
err:= d.Deliver(ctx, hook, map[string]any{
 "type": "process.crashed",
 "id": processID,
})
// or: webhook.Deliver(ctx, hook, payload)
```

Delivery rules:

1. Run SSRF check (`SSRFCheck` or `ValidateURL`).
2. Marshal payload as JSON.
3. `POST` with `Content-Type: application/json` and `User-Agent: pmmcp-webhook/1`.
4. Drain response body (up to `MaxBody`); require **2xx** status.

Non-2xx returns an error with the status code. Network/marshal failures wrap with `webhook: …`.

#### Testing with httptest

Default policy **blocks loopback**, so `httptest` URLs fail under production checks. Tests inject a looser `SSRFCheck`:

```go
d:= &webhook.Deliverer{
 Client: srv.Client,
 SSRFCheck: func(raw string) error {
 if err:= webhook.ValidateURL(raw); err != nil {
 if errors.Is(err, webhook.ErrSSRF) {
 return nil // allow loopback for this test only
 }
 return err
 }
 return nil
 },
}
```

Never ship a production `Deliverer` that skips SSRF by default.

## SSRF policy

`ValidateURL` enforces:

| Check | Result |
|-------|--------|
| Empty / unparseable URL | `ErrInvalid` |
| Scheme not `http` or `https` | `ErrSSRF` (e.g. `file:`, `ftp:`) |
| Empty host | `ErrInvalid` |
| Blocked hostname | `ErrSSRF` |
| Literal IP in blocked class | `ErrSSRF` |
| DNS resolves to blocked IP | `ErrSSRF` |
| DNS lookup fails | **Allowed** at validation (register OK; dial fails later) |

### Blocked hostnames

- `localhost`, `localhost.localdomain`
- any `*.localhost` suffix
- `metadata`, `metadata.google.internal`

### Blocked IP classes

- Loopback (`127.0.0.0/8`, `::1`)
- Link-local unicast/multicast (includes `169.254.0.0/16`)
- Unspecified (`0.0.0.0`, `::`)
- Explicit cloud metadata IPv4 `169.254.169.254` (also covered by link-local)

### Not blocked by this package

- General RFC1918 private ranges (`10.0.0.0/8`, etc.) — operators may deliberately target internal receivers. Product-level allowlists can narrow this further in daemon config without changing these primitives.

### Gaps vs full 

Implemented now:

- Scheme restriction, metadata/loopback/link-local blocks, best-effort resolve-and-check, short timeouts, response size drain.

Deferred / higher-layer:

- Configurable URL/host allowlist
- Redirect non-following guarantees (stdlib client may still redirect within policy unless configured)
- HMAC payload signing
- Retries with backoff
- Secret redaction in payloads (caller responsibility)

## Errors

| Sentinel | When |
|----------|------|
| `ErrInvalid` | Empty id/url, parse failures |
| `ErrNotFound` | Unknown hook id |
| `ErrSSRF` | Policy-blocked destination |

Always compare with `errors.Is`.

## Who imports this package

| Importer | Usage |
|----------|--------|
| `internal/daemon` (`server_state.go`) | Owns `*webhook.Registry` in product state |
| `internal/daemon` (`handlers_ext.go`) | webhook create/update/delete/list/test IPC; maps sentinels to API error codes |
| `internal/daemon` (`server.go`) | Holds `hooks` field on `Server` |

Typical daemon mapping:

- `ErrSSRF` / `ErrInvalid` → invalid argument
- `ErrNotFound` → not found
- Audit actions: `webhook.create`, `webhook.update`, `webhook.delete`, `webhook.test`

## Files

| File | Contents |
|------|----------|
| `doc.go` | Package comment + pointer |
| `webhook.go` | Registry, Deliverer, ValidateURL |
| `webhook_test.go` | CRUD, SSRF matrix, deliver allow/deny |

## Testing

```bash
go test./internal/webhook/ -count=1
```

Covers registry CRUD, metadata/loopback rejection at create and deliver, and a successful POST path with injected SSRF check.

## Security notes

- Treat every webhook URL as **agent-influenced config** until proven otherwise.
- Prefer fail-closed changes: broaden blocks freely; widen allow only with explicit product decision and tests.
- Do not log full payloads if they may include env/secret material (redact at the event construction site).
- Webhooks must not become an inbound API; see control-plane split and threat model.
