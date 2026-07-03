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

