# agents: declare

## role
Parse, **validate**, and diff **`pmmcp.yaml`** documents (`apiVersion: pmmcp.dev/v1alpha1`, kind Project). Enforces security policy (sandbox defaults, shell-risk argv, privileged ports, watch path containment, webhook SSRF). Procfile import produces explicit argv or visible `["/bin/sh","-c",…]`.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Document`, `Spec`, `ServiceSpec`, `WebhookSpec`, `PortSpec`, `WatchSpec` | YAML models |
| `CanonicalAPIVersion` | `pmmcp.dev/v1alpha1` |
| `Parse` / `ParseStrict` | Strict uses yaml KnownFields |
| `(*Document).Validate(opts...)` | Structural + policy; collects `ValidationErrors` |
| Validate options | `WithProjectRoot`, `WithAllowSandboxOff`, `WithAllowShellArgv`, `WithAllowPrivilegedPorts`, `WithWebhookAllowlist` |
| `DiffServices` | create/update/delete/noop vs running names |
| `ImportProcfile` | Split plain argv or wrap shell metachar commands as `/bin/sh -c` (not `-lc`) |
| `ErrInvalid`, policy codes | Hostile fixtures (e.g. 023) rejected wholesale |

## deps
- Project: none (policy helpers local; webhook URL rules aligned with webhook package intent)
- Third-party: `gopkg.in/yaml.v3`

## invariants
- Default Validate is **deny-by-default** security policy; relaxations are explicit options (capability-gated at daemon).
- Shell-risk argv flagged unless `WithAllowShellArgv`.
- Sandbox `off` denied unless allowed; webhooks subject to SSRF/allowlist checks.
- Watch paths must stay under project root when root is set.
- No secret values in YAML — refs only (validated as structure, not resolved here).
- Procfile shell wrap is **visible** in argv (reviewable).

## tests
- `declare_test.go`, `policy_test.go` — samples, hostile 023, SSRF variants, procfile, depends_on forms.
- Unit tests hermetic (`t.Parallel()` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## touch map
| Change | Also touch |
|--------|------------|
| apiVersion / schema fields | docs, examples, daemon apply handlers, CLI declare verbs |
| Policy defaults (sandbox/shell/ports/webhooks) | SECURITY.md, authz capability gates for relax options |
| ImportProcfile argv rules | docs import-compose/procfile |
| Diff semantics | `pm_diff` / apply UX |

## do-not
- Auto-apply without daemon authz.
- Silently drop unknown fields when using ParseStrict callers.
- Store or resolve secret values in this package.
- Soften SSRF/webhook checks without product review.

## related
- `internal/daemon` declare handlers, `internal/webhook`, `internal/authz` CapDeclareApply
