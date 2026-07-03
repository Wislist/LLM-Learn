# Tools

`internal/agent/tools` contains the first-pass coding tool set for the agent.

Current scope:

- `bash`: shell command execution tool and prompt template
- `read`: read workspace files
- `write`: create or overwrite files
- `edit`: exact string replacement
- `ls`: list workspace directories
- `glob`: find files by glob
- `grep`: search text in files
- `job_output`: read background job output
- `job_kill`: stop background jobs

This package currently defines names, defaults, prompt templates, and the first concrete tool implementation.
The remaining concrete tools and runtime/TUI execution interface will be added in later tasks.

`bash` is now implemented as the first concrete tool. It currently uses
`os/exec` with `bash -lc`, not `mvdan/sh`. It supports:

- command validation
- banned command checks
- working directory validation
- output truncation
- foreground execution
- explicit background execution
- auto-backgrounding after a configured duration
- job id/status metadata for future TUI rendering

## Tool interface

Agent tools expose a `ToolDefinition` and run with structured input/output:

- `ToolDefinition`: name, description, JSON schema, prompt text, behavior flags
- `ToolBehavior`: dangerous, requires confirmation, supports background, read-only
- `ToolInput`: call id, tool name, raw JSON arguments
- `ToolOutput`: textual content plus metadata for UI/runtime consumers

The metadata channel is reserved for values such as `cwd`, `exit_code`, `job_id`, truncation flags, and other tool-specific fields.

## Prompt rendering

Tool descriptions live beside the tool package as `.md` or `.md.tpl` files.

`RenderToolPrompt` loads the matching file by tool name:

- static `.md` files are returned as trimmed text
- `.md.tpl` files are rendered with `PromptData`

`DefaultPromptData` currently provides:

- banned command list
- max output length
- max result count
- whether `rg` is available

Concrete tools should put the rendered prompt into `ToolDefinition.Prompt` when they are implemented.
