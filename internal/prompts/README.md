# package `prompts`

Single home for **agent-facing prompt and description text**.

## Layout

| File | Role |
|------|------|
| **`lines.toml`** | Every single-line description: tools, resources, prompt meta, arg help |
| **`md/*.md`** | Multi-line MCP prompt bodies + static resource docs |

```
internal/prompts/
  lines.toml          # tools / resources / prompt_desc / prompt_arg
  lines.go            # load + ToolDescription / Resource*
  prompts.go          # catalog structure + Render
  md/
    pmmcp_*.md        # prompt bodies ({{arg}} placeholders)
    docs_*.md         # pmmcp://docs/* bodies
```

## Single-line API

```go
prompts.ToolDescription("pm_start")
prompts.ToolDescriptions() // copy of all tools → cli.ToolDescription

prompts.ResourceDescription("processes")
prompts.ResourceTemplateDescription("process")
prompts.ResourceDynDescription(prompts.DynProcessStatus, "web")
// → "Process web status"

prompts.PromptDescription("pmmcp_start_safe")
prompts.PromptArgDescription("pmmcp_start_safe", "name")
```

## Multi-line API

```go
prompts.List()                          // Specs with hydrated descriptions
prompts.Render("pmmcp_start_safe", args)
prompts.Doc(prompts.DocErrorCodes)
```

## Editing copy

1. **Tool one-liner** → `[tools]` in `lines.toml`
2. **Resource / template description** → `[resources]` / `[resource_templates]`
3. **Dynamic list description** → `[resource_dyn]` with `{{name}}`
4. **Prompt list description / arg help** → `[prompt_desc]` / `[prompt_arg]`
5. **Prompt body** → `md/<name>.md`

Then rebuild. No need to touch `catalog.go` or `resources.go` for wording.

## Adding a tool description

1. Register the tool in `cli.ToolMethod`.
2. Add `pm_… = "…"` under `[tools]` in `lines.toml`.
3. `TestToolDescriptionCoversAll` fails if either side is missing.
