# Tools

`internal/agent/tools` contains the first-pass coding tool set for the agent.

Current scope:

- `bash`: shell command execution prompt template
- `read`: read workspace files
- `write`: create or overwrite files
- `edit`: exact string replacement
- `ls`: list workspace directories
- `glob`: find files by glob
- `grep`: search text in files
- `job_output`: read background job output
- `job_kill`: stop background jobs

This package currently defines names, defaults, and tool prompt templates only.
The concrete tool implementations and runtime/TUI execution interface will be added in later tasks.

## Tool interface

Agent tools expose a `ToolDefinition` and run with structured input/output:

- `ToolDefinition`: name, description, JSON schema, prompt text, behavior flags
- `ToolBehavior`: dangerous, requires confirmation, supports background, read-only
- `ToolInput`: call id, tool name, raw JSON arguments
- `ToolOutput`: textual content plus metadata for UI/runtime consumers

The metadata channel is reserved for values such as `cwd`, `exit_code`, `job_id`, truncation flags, and other tool-specific fields.
