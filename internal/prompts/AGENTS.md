# agents: prompts

## role
Central store of agent-facing copy: single-line tool/resource/prompt metadata in embedded `lines.toml`, multi-line MCP prompt bodies and static docs under embedded `md/*.md`. Callers must not hard-code description strings.

## surface
| Symbol / area | Notes |
|---------------|--------|
| `Spec`, `Arg` | Prompt catalog structure (names, required, defaults, file) |
| `List`, `Lookup`, `Render` | Catalog + `{{arg}}` body fill; unknown → `domain.CodeNotFound` |
| `Doc`, `MustDoc`, `DocErrorCodes`, `DocToolIndex` | Static markdown resources (no substitution) |
| `ToolDescription`, `ToolDescriptions` | tools/list one-liners; map is a **copy** |
| `ResourceDescription`, templates, dyn | Resource copy; dyn keys `DynProcessStatus` / `DynProcessLog` / `DynGroupStatus` |
| `PromptDescription`, `PromptArgDescription` | prompts/list and arg help from TOML |

## deps
- Project: `internal/domain`
- Third-party: `github.com/BurntSushi/toml` (load-bearing for `lines.toml`)

## invariants
- Wording lives in `lines.toml` / `md/*.md`, not Go string tables.
- Prompt **structure** (required args, defaults, file basenames) stays in the Go `catalog`.
- `Doc` / `readMD` accept basenames only — reject `..` and path separators.
- Placeholders are `{{key}}`; missing/empty args use `Arg.Default` when set.
- Copy must not instruct agents to dump secrets or casually disable sandbox (root security).

## tests
- `prompts_test.go` — list/lookup/render, docs path safety, lines maps, substitute behavior.
- Unit tests hermetic (`t.Parallel()` when safe). **Coverage floor: ≥80% statements** for this package (CI). Do not add production seams only to hit residual lines. Meaningful assertions beat line hits.

## touch map
- New tool description → `lines.toml` `[tools]` (and cli catalog if new tool).
- New MCP prompt → `catalog` entry + `md/*.md` + `prompt_desc`/`prompt_arg` in TOML.
- Static MCP resource docs → `md/` + resource registration in `internal/mcp`.

## do-not
- Hard-code agent-facing description strings in `cli` or `mcp`.
- Put MCP SDK, IPC, or daemon logic in this package.
- Allow path traversal in `Doc` filenames.
- Ship prompt bodies that tell agents to exfiltrate secrets or disable peer-cred/sandbox.

## related
Importers: `internal/mcp`, `internal/cli` (`ToolDescriptions` / tools/list).
