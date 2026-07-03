# Tools

`internal/agent/tools` contains the first-pass coding tool set for the agent.

Current scope:

- `bash`: shell command execution tool and instruction file
- `read`: read workspace files
- `write`: create or overwrite files
- `edit`: exact string replacement
- `ls`: list workspace directories
- `glob`: find files by glob
- `grep`: search text in files
- `job_output`: read background job output and status
- `job_kill`: stop background jobs

This package currently defines names, defaults, instruction files, and concrete shell/job tools.
The remaining file/search tools and runtime/TUI execution interface will be added in later tasks.

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

`job_output` and `job_kill` share the same `JobManager` used by `bash`, allowing callers to inspect or terminate background commands by `job_id`.

## Tool interface

Agent tools expose a `ToolDefinition` and run with structured input/output:

- `ToolDefinition`: name, description, JSON schema, prompt text, behavior flags
- `ToolBehavior`: dangerous, requires confirmation, supports background, read-only
- `ToolInput`: call id, tool name, raw JSON arguments
- `ToolOutput`: textual content plus metadata for UI/runtime consumers

The metadata channel is reserved for values such as `cwd`, `exit_code`, `job_id`, truncation flags, and other tool-specific fields.

## Instruction rendering

Tool instructions live beside the tool package as `.md` or `.md.tpl` files.

`RenderToolInstructions` loads the matching file by tool name:

- static `.md` files are returned as trimmed text
- `.md.tpl` files are rendered with `InstructionData`

`DefaultInstructionData` currently provides:

- banned command list
- max output length
- max result count
- whether `rg` is available

Concrete tools should put the rendered instructions into `ToolDefinition.Prompt` when they are implemented.
