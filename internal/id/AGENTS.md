# agents: id

## role
Generate, parse, and validate prefixed lowercase Crockford-base32 ULIDs for all durable public record IDs. Single owner of the prefix vocabulary.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Prefix` + consts | `Proc`, `Group`, `Profile`, `Event`, `Audit`, `Session`, `Project` |
| `New(prefix)` | `<prefix>-<26-char lowercase ULID>` via `crypto/rand` |
| `Parse(s)` | Returns prefix + `ulid.ULID`; rejects non-canonical input |
| `Valid`, `HasPrefix` | Thin wrappers over `Parse` |
| `ErrInvalid` | Sentinel for unknown/malformed IDs |

## deps
- Project: none (leaf)
- Third-party: `github.com/oklog/ulid/v2` (load-bearing)

## invariants
- Canonical form only: lowercase prefix, hyphen, 26-char lowercase body (no hyphens in body).
- `Parse` does **not** trim or rewrite case — uppercase/padded input is rejected so map/SQL keys match exact strings.
- Entropy from `crypto/rand.Reader` via `ulid.New`; never `math/rand`.
- Unknown prefix on `New`/`Parse` → `ErrInvalid`.
- Adding a resource type requires a new `Prefix` const **and** an entry in `known`.

## tests
- `id_test.go` — round-trip all prefixes; reject empty, wrong length, multi-hyphen, case, unknown prefix.
- Unit tests hermetic (`t.Parallel` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## touch map
- New resource ID type → `Prefix` const + `known` + callers that mint IDs.
- Parse strictness changes → review store keys and exact-string equality sites.

## do-not
- Use UUID / UUIDv7 for new public resource PKs.
- Invent ad-hoc prefixes outside this package.
- Encode path, port, or human name into the ULID body.
- Accept unprefixed bare ULIDs as valid public IDs.
- Switch entropy to `math/rand`.

## related
Importers: `daemon`, `group`, `event`, `audit`, `session`, `profile`, store tests.
