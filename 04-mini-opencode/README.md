# mini-opencode

一个从零开始实现的 Go 版 agent 终端项目。

本项目不依赖仓库里已有的 `02-llmg`、`03-mini-opencode` 或其它历史实现。

## 目标

实现一个接近 `crush` 一半能力的本地 agent 终端：

1. Bubble Tea TUI
2. Agent runtime event stream
3. 权限与工具执行体验
4. 上下文工程
5. 多 provider 配置
6. MCP lifecycle
7. Git diff 与测试反馈

## 当前阶段

当前已经搭好第一层 agent 工作流：

- 独立 `go.mod`
- 独立入口 `cmd/mini-opencode`
- 标准库 CLI 循环
- `internal/agent` runtime
- provider 抽象
- tool 抽象与注册表
- `internal/agent/tools` 常用编码工具模板
- event stream
- 基础 turn loop：用户输入 -> provider -> assistant message -> tool call -> tool result -> 继续推理
- MCP stdio client 与 tool adapter
- 后续再逐步加入 provider、tools、TUI 和 MCP

## 目录

```text
cmd/mini-opencode     CLI 入口
internal/app          当前标准库 CLI 壳
internal/agent        agent 核心工作流
internal/agent/prompt Prompt 组装与模板
internal/agent/tools  常用编码工具模板
internal/mcp          MCP stdio client 与工具适配
docs/mcp.md           MCP 接入说明
docs/prompt.md        Prompt 组装说明
docs/tools.md         Tools 说明
docs/providers.md     Provider 配置说明
```

## 运行

```bash
go run ./cmd/mini-opencode
```
