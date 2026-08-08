# agents: webhook

## role
Outbound webhook **registry + SSRF-safe delivery**. POSTs JSON only to admitted http/https destinations; empty allowlist means webhooks disabled.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Hook` | ID, URL, Events filter, Secret (HMAC key; not serialized out) |
| `Registry`, `NewRegistry`, `WithAllowlist` | Create/List/Get/Delete; admission on Create |
| `Deliverer`, `Deliver` / `DeliverEvent` | POST JSON; `X-PMMCP-Event-Type`, delivery id; optional `X-PMMCP-Signature: sha256=…` |
| Sentinels | `ErrNotFound`, `ErrInvalid`, `ErrSSRF` |
| SSRF policy | Block loopback, link-local, unspecified, cloud metadata (`169.254.169.254`); resolve+pin at dial; revalidate redirects; resolver failure fails closed |

## deps
- Project: none
- Third-party: `github.com/oklog/ulid/v2` (delivery IDs)

## invariants
- **Empty allowlist → Create denied** (webhooks off by default).
- Only http/https; hostname and resolved IP checked; dial pins vetted IPs (DNS rebinding safe).
- Redirects re-validated; hop limit enforced.
- HMAC uses `crypto/hmac` + SHA-256; secrets not logged.
- Context on every outbound request.

## tests
- `webhook_test.go`, `webhook_internal_test.go`, `deliver_internal_test.go` — CRUD, SSRF tables, allowlist, dial pin, signature.
- Unit tests hermetic (`t.Parallel` when safe; httptest for deliver). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## touch map
| Change | Also touch |
|--------|------------|
| SSRF blocklists / allowlist semantics | `declare` webhook policy, daemon dispatch, SECURITY/docs |
| Signature headers | External consumer docs |
| Registry API | daemon handlers + CLI/MCP catalog if exposed |

## do-not
- Default-allow loopback/metadata “for local dev” in production code paths.
- Log Hook.Secret or raw signing keys.
- Open a public inbound webhook listener here (outbound only).

## related
- `internal/declare` webhook validation, `internal/daemon` webhook_dispatch
