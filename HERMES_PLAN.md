# Hermes 工程学习计划（本地 Agent 开发 · Go 主力 · 3 个月+ · 每天 4h）

> 仓库地址：git@github.com:Wislist/LLM-Learn.git
> 起始日期：2026-06-28
> 核心模型：Nous Research Hermes-2 Pro / Hermes-3（本地部署）

---

## 总路线图

```
Month 1 → Hermes 模型认知 + 本地部署 + Function Calling 实战    （产出：本地 Tool-calling 服务）
Month 2 → Agent 框架 + 多工具编排 + MCP 集成                    （产出：Hermes Agent 框架 v0）
Month 3 → 生产化：RAG + Memory + 评测 + 微调                    （产出：可交付的本地 Agent 平台）
Month 4+ → 多模态 / App 化 / 社区贡献
```

核心目标：**用 Go + 本地 Hermes 模型，从零构建一个私有化部署的 Agent 平台**。全程不依赖商业 API，掌握模型侧 + 工程侧两端能力。

---

## 为什么选 Hermes？

| 维度 | Hermes 的优势 |
|---|---|
| Function Calling | Hermes-2 Pro 引入专用 tool-call token（`<tool_call>`），解析极其稳定 |
| 结构化输出 | 原生 JSON 模式，配合 grammar 约束几乎不会格式乱飞 |
| 本地可控 | 7B/8B 即可跑动，MacBook 32G 轻松带起，无需 GPU 服务器 |
| 开源生态 | Nous Research 持续维护，Ollama/vLLM/llama.cpp 全支持 |
| 系统提示 | 开源了完整的 Hermes system prompt，可参考其多工具编排逻辑 |
| Agent DNA | 从 NeurIPS 开始就定位为 agent 专用模型，不是通用 chat 模型缝补 tool use |

相比之下，DeepSeek/OpenAI 更适合作为"对标参考"而不是主力——因为你无法掌控模型侧。Hermes 让你从 Tokenizer 到 System Prompt 到 Inference Pipeline 全链路可控。

---

## Month 1：Hermes 认知 + 本地部署 + Function Calling 实战

### Week 1 — 摸透 Hermes
- 理论：
  - Nous Research 的技术博客，理解 Hermes 从 Hermes-1 → Hermes-2 Pro → Hermes-3 的演进
  - 精读 Hermes-2 Pro 的 system prompt（GitHub 开源），拆解其 tool-call 编排逻辑
  - 理解 `<tool_call>` token 的设计：为什么不用标准的 OpenAI function calling JSON
  - 对比：Hermes tool-use vs GPT-4 function calling vs Claude tool_use — 三套 schema 的差异
- 动手：
  - 在 Ollama 拉取 `hermes3:8b` 和 `hermes2-pro-mistral:7b`
  - 用 `curl` 直连 Ollama API，测试 chat 和 tool call
  - 观察不同温度下的行为差异（Hermes 在 temp=0 时 tool call 最稳定）
  - 用 `llama.cpp` 的 server 模式对比推理性能
- 产出：`hermes-playground` — 一个交互式测试工具，能切换模型、调参数、观察 tool call 原始输出

### Week 2 — 深入 Tool Calling
- Hermes tool call 格式精要（以 Hermes-2 Pro 为例）：
  ```
  <tool_call>
  {"name": "get_weather", "arguments": {"city": "Beijing"}}
  </tool_call>
  ```
  - 模型会在生成中直接插入 XML 风格的 tool-call 块
  - 解析：用 Go 的 `encoding/xml` + JSON 二次解析，比正则稳健
- 实战任务：
  - 手写一个 Go 库 `hermestool`，封装 Hermes tool-call 的解析和构造
  - 支持多工具并行调用（Hermes-3 原生支持一次返回多个 `<tool_call>`）
  - 处理边界：不完整的 tool call（流式输出截断）、嵌套 JSON 内的特殊字符
- 阅读：
  - Hermes tool-use benchmark（BFCL / Berkeley Function Calling Leaderboard 上的表现）
  - 社区整理的 Hermes tool-call prompt template 合集
- 产出：`hermestool` Go 包，能可靠解析单/多 tool call，含完整单测

### Week 3-4 — 第一个 Hermes Agent Loop
- ReAct + Hermes 的适配：
  - Hermes 的 system prompt 天然就是 ReAct 风格（Observation → Thought → Action）
  - 需要加一个"tool result" 注入机制——把工具执行结果拼回对话
  - 会话历史超出上下文窗口时的截断策略（保留最近 N 轮 + 关键 tool call）
- Go 实现：
  - 流式 SSE 解析（Ollama API 兼容 `/api/generate` 和 `/api/chat`）
  - Agent loop：`parse → think → tool_detect → execute → inject_result → continue`
  - 停止条件：模型不再输出 `<tool_call>` 且无进一步追问
- 工具生态：
  - 文件读写（本地项目操作）
  - Bash 执行（带进程沙箱，用 Go 的 `os/exec` + `unshare`）
  - HTTP 请求（带超时和白名单）
  - 代码搜索（grep / find 封装）
- 交互：
  - CLI UI：用 `bubbletea` 做一个 TUI
  - 命令：`/help`、`/clear`、`/model`（切模型）、`/tool`（列出工具）
- **M1 产出**：`hermes-cli-agent` v0.1
  - 完全本地运行（Ollama + Hermes-3），不依赖任何商业 API
  - 能自动读写文件、执行 bash、搜索代码
  - 附一个 5 分钟的录屏 demo

---

## Month 2：Agent 框架 + 多工具编排 + MCP

### Week 5 — Agent 模式设计
- 精读论文（无需全懂，但要能讲出核心思路）：
  - ReAct: Synergizing Reasoning and Acting in Language Models
  - Toolformer / Gorilla / APIGen（模型如何学会用工具）
  - Plan-and-Execute vs ReWOO vs LLMCompiler（编排范式对比）
- 实现多种 Agent 模式：
  - **SimpleReAct**：标准的 tool-use loop（已实现）
  - **PlanAgent**：先让 Hermes 出计划清单，再逐步执行
  - **WorkerPool**：主 agent 调度 + 多个 worker agent（各自独立上下文）
- Go 设计：
  - `Agent` interface：`Run(ctx, task) → Result`
  - `Tool` interface：`Name() / Description() / Schema() / Execute(ctx, args)`
  - `Memory` interface：`Add(msg) / Get(n) / Summarize(ctx)`
  - 每种 Agent 模式是一个 `Agent` 实现
- 产出：`hermes-agent-core` v0.1 — 支持 3 种 Agent 模式的框架核心

### Week 6 — MCP 集成
- 精读 MCP spec：https://modelcontextprotocol.io
- 用 Go 写一个 MCP server：
  - 参考 `mark3labs/mcp-go`，不要太早自己造轮子
  - 把 Week 3-4 的工具（文件/代码/bash/web）包装成 MCP server
  - 支持 stdio transport（用于 Claude Desktop / opencode 等客户端）
- 在 `hermes-cli-agent` 中加一个 MCP client：
  - 自动发现和加载 MCP server
  - 把 MCP tools 转换成 Hermes 的 tool format
  - 让 Hermes agent 能用外部 MCP 工具
- 测试：启动本地的 MCP file-server，让 Claude Desktop 和你的 agent 都能读写同一个沙箱目录
- 产出：`hermes-mcp-bridge` — MCP server/client 工具集

### Week 7 — RAG + 本地知识库
- Embedding 模型选型：`bge-m3`（多语言/8192 token）或 `nomic-embed-text`（轻量）
- 向量库：Qdrant（Docker 本地部署，Go client 直连）
- Chunking 策略：
  - Markdown 按 `##` 标题分段
  - 代码按函数/类边界分块（用 tree-sitter）
  - 固定 overlap 兜底
- 检索 + Rerank：
  - 第一阶段：向量召回 top-30
  - 第二阶段：用 Hermes 本身做 rerank（"以下文档与问题的相关性打分 0-10"）
- 工具化：`search_knowledge(query)` → 返回带 chunk-id 的 top-k
- 引用回溯：要求 Hermes 在回答时标注引用 id
- 语料：Go 标准库文档 / 你自己的笔记 / Hermes 相关论文
- 产出：`hermes-rag` 模块，接入 agent 后能基于本地文档回答技术问题

### Week 8 — 可观测性 + 评测框架
- Trace：
  - Langfuse 自部署（Docker Compose），接 Go SDK
  - 记录每一轮：输入 token 数、生成 token 数、延迟、tool-call 详情
  - 可视化面板：看 agent 在哪一步花费最多时间
- Agent 评测（Eval）：
  - 从 SWE-bench 里抽 20 个简化版任务（如"在某个文件里加一个函数"）
  - 指标：成功率 / 平均步数 / 总 token / 幻觉率（引用不存在的文件/函数）
  - Hermes 专门的评测集：BFCL（Function Calling）上的对应任务
- 自动回归：每次改 prompt 或升级模型后跑一遍 eval
- **M2 产出**：`hermes-agent-framework` v1.0
  - 多 Agent 模式 + MCP + RAG + Trace + Eval，五件套齐全
  - README 完整，附 3 个 demo 场景的录屏

---

## Month 3：生产化 + 微调 + 生态

### Week 9 — Memory & 长期记忆
- 短期记忆：Token 窗口管理 + 滑动截断
- 长期记忆：
  - 每轮对话结束后提取摘要 → embedding → 存入 Qdrant
  - 新对话开始时语义召回相关历史
- 用户档案：偏好记录（"用户喜欢用 Go 而不是 Python"、"项目目录在 X"）
- 让 Hermes 在需要时主动调用 `recall_memory` 工具
- 产出：`hermes-memory` 模块，agent 能跨对话记住上下文

### Week 10 — 微调 Hermes 做专用 Agent
- 这一步是"模型侧"的核心进阶：
- 环境：租一个单卡 H100（约 $2/h）或 Colab Pro+
- 框架：Axolotl / Unsloth（推荐后者，Q-LoRA 微调 7B/8B 只需 ~12GB VRAM）
- 数据准备：
  - 从你自己的 agent 日志中提取 tool-call 成功/失败 case
  - 用 Hermes 3 合成更多 tool-use 训练数据（Self-Instruct 思路）
  - 目标格式：Align 到 Hermes chat template
- 微调目标：
  - 降低工具格式错误率（解析失败率从 ~5% 降到 <1%）
  - 让模型更好地遵循自定义 system prompt
- 评估：用 BFCL 和自建 eval 集对比微调前后
- 产出：`hermes-myagent-lora`（一个 LoRA 权重，开源到 Hugging Face）

### Week 11 — 多模态（轻量）
- 接入 Hermes 的 vision 分支（如果有，或使用 Llama-3.2-Vision 做图片理解）
- 工具：`screenshot_capture` + `analyze_image` — 让 agent 能"看"屏幕
- 场景：你截一张报错图 → agent 分析排版/颜色/报错信息 → 定位代码
- 产出：visual-agent demo

### Week 12 — 打包发布
- 用 Wails（Go + 前端）包装成 macOS 桌面 App
  - 左侧：对话面板
  - 右侧：文件浏览器 + 终端
  - 底部：工具调用日志
- 写技术博客系列（3 篇）：
  1. 「为什么选 Hermes 作为 Agent 基座」
  2. 「从零写一个本地 Agent 框架」
  3. 「微调 Hermes 做专用 Agent 的全流程」
- 发 GitHub + V2EX + Twitter
- **M3 产出**：可下载使用的桌面 Agent App + 开源框架 + 微调模型 + 博客

---

## Month 4+（可选的深水区）
- 接入 SeaLLM / Qwen 做中英双语 agent（Hermes 英文强，中文可以联合作战）
- 自部署 vLLM 推理服务，做并发压测（100+ QPS 场景）
- 参与 Hermes 社区：给 Hermes-4 提 tool-use benchmark 结果
- 做成 SaaS：把 agent 能力包装成 API，部署到 Fly.io / Railway

---

## 推荐技术栈（Hermes 工程专用）

| 用途 | 选型 | 理由 |
|---|---|---|
| 主力模型 | Hermes-3 8B（Ollama 本地） | Tool-call 最稳的开源模型 |
| 备用模型 | Hermes-2 Pro Mistral 7B | 轻量，测试低配场景 |
| 推理服务 | Ollama（开发期）→ vLLM（生产期） | 从简单到高性能的渐进路径 |
| Go 语言 | 全程 | 性能好，部署二进制，agent 首选 |
| 向量库 | Qdrant（Docker 本地） | Go client 质量最高 |
| Embedding | bge-m3 / nomic-embed-text | Ollama 直接拉取，零配置 |
| Trace | Langfuse（Docker 本地） | 开源，部署简单，Go SDK 完善 |
| Agent 协议 | MCP over stdio | 社区标准，与 Claude Desktop / opencode 互通 |
| 微调框架 | Unsloth（Q-LoRA） | 内存效率最高，7B 仅需 12GB VRAM |
| 桌面壳 | Wails（Go + Vue/Svelte） | 比 Electron 轻 10 倍，Go 全栈 |
| 部署 | Docker Compose（开发）→ Fly.io（生产） | 从本地到云的无缝迁移 |

---

## 每日 4h 时间分配建议

- **1h**：读论文 / 文档 / 社区帖（Hermes 相关优先）
- **2h**：写 Go 代码 / 调试 agent
- **1h**：记笔记（Markdown）+ 录 demo 片段

---

## 仓库目录规划（在 LLM-Learn 内）

```
LLM-Learn/
├── LEARNING_PLAN.md              # 通用 LLM 学习路线
├── HERMES_PLAN.md                # 本文件（Hermes 专项路线）
├── notes/
│   ├── hermes-week-01.md         # Hermes 学习笔记
│   └── ...
├── 01-hermes-playground/         # Week 1：模型测试工具
├── 02-hermestool/                # Week 2：Tool-call 解析库
├── 03-hermes-cli-agent/          # Week 3-4：CLI Agent
├── 04-hermes-agent-core/         # Week 5：Agent 框架核心
├── 05-hermes-mcp-bridge/         # Week 6：MCP 集成
├── 06-hermes-rag/                # Week 7：本地 RAG
├── 07-hermes-eval/               # Week 8：评测框架
├── 08-hermes-memory/             # Week 9：长期记忆
├── 09-hermes-finetune/           # Week 10：微调
├── 10-hermes-desktop/            # Week 12：桌面 App
└── docs/                         # 论文笔记 / 参考文章
```

---

## 核心参考资料

### Hermes 模型 & 工具调用
- Nous Research 官方博客：https://nousresearch.com
- Hermes-2 Pro 技术报告 + System Prompt（GitHub: `NousResearch/Hermes-Function-Calling`）
- Hermes-3 发布公告 + Chat Template
- BFCL Leaderboard（Berkeley Function Calling）：https://gorilla.cs.berkeley.edu/leaderboard.html

### Agent 理论
- ReAct 论文：Yao et al., "ReAct: Synergizing Reasoning and Acting in Language Models"
- Toolformer / Gorilla / APIGen 系列
- Plan-and-Execute / ReWOO / LLMCompiler 对比

### 协议 & 生态
- MCP 协议规范：https://modelcontextprotocol.io
- Ollama 文档：https://ollama.com
- Qdrant Go client：https://github.com/qdrant/go-client
- Langfuse 文档：https://langfuse.com
- Unsloth 微调：https://github.com/unslothai/unsloth

### Go 生态参考
- `mark3labs/mcp-go`：Go 的 MCP 实现
- `tmc/langchaingo`：Go 的 LangChain 等价物（参考架构思路，不建议全量依赖）
- `charmbracelet/bubbletea`：Go TUI 框架
- `wailsapp/wails`：Go 桌面 App 框架

### 对标项目（Inspiration）
- opencode（Go 实现的 CLI agent）— 你 M1 路线也在参考它
- Open Interpreter — Python 生态的本地 agent 标杆
- Goose（Block 开源的本地 agent）— MCP + 多 Agent 架构值得学习

---

## 与 `LEARNING_PLAN.md` 的关系

| 维度 | LEARNING_PLAN.md（通用路线） | HERMES_PLAN.md（本文件） |
|---|---|---|
| 模型 | DeepSeek / OpenRouter（云端 API） | Hermes（本地部署） |
| 核心命题 | 通用 LLM 应用开发 | Hermes 专项 agent 工程 |
| 模型侧深挖 | 不涉及 | 微调 + system prompt 调优 + inference 优化 |
| 独立性 | 自己写 llmg 库 | 自己写 hermestool + agent 框架 |
| 最终形态 | 云 API 驱动的 CLI agent | 完全本地化的桌面 agent |

两条线可以并行：M1 用 LEARNING_PLAN 走 cloud-first 路线快速出东西，同时用 HERMES_PLAN 在本地深耕 Hermes。大约在 Week 5-6 左右会自然交汇（Agent Loop 和 MCP 是相通的）。

---

> **记住两句话：**
> 1. "Hermes 的价值不在模型权重，在于它把 tool-use 做成了原生能力，而不是 prompt hack。"
> 2. "本地 agent 的意义不是省钱，是可控。你能改 tokenizer、能调 system prompt、能微调、能离线，这才是工程师的护城河。"

Happy hacking, with Hermes 🧠
