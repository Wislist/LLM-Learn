# LLM 应用 & Agent 开发学习计划（Go 主力 · 3 个月+ · 每天 4h）

> 仓库地址：git@github.com:Wislist/LLM-Learn.git
> 起始日期：2026-06-28

---

## 总路线图

```
Month 1 → 应用基础 + CLI Agent         （产出：mini-opencode v0）
Month 2 → RAG + 知识库                 （产出：带引用的知识助手）
Month 3 → Agent 编排 + MCP + 可观测    （产出：多工具 Agent 平台）
Month 4+ → 进阶：Function calling 续、多模态、评测上架
```

核心目标：**用 Go 从零写一个自己的 opencode**。3 个月后 GitHub 上会有 4 个 portfolio 项目。

---

## Month 1：应用基础 + 第一个 CLI Agent

### Week 1 — 理论快车道（每天约 2h 理论 + 2h 折腾）
- 必看：Karpathy「Let's build GPT」、3B1B Transformer 可视化
- 必懂视角（不炼丹但要会讲）：
  - token / KV cache / 上下文窗口
  - 采样参数（temp / top_p / top_k）/ 停止条件
  - 预训练 vs SFT vs RLHF 的关系
- 动手：申请 DeepSeek / OpenRouter / WRAP key，用 curl + `net/http` 手搓一次性请求，观察各种参数差异
- 产出：一个 `hello-llm` 小 demo，能打印流式响应

### Week 2 — API 与 Prompt 工程
- Prompt 结构：System / User / Assistant 三段、Few-shot
- JSON 输出 coerce 三种路线对比：JSON Schema、Tool calling、Grammar（GBNF）
- 阅读：Anthropic 的 prompt engineering guide、DeepSeek-V3 system prompt（已开源，看它怎么编排工具）
- **Go 项目**：`llmg` —— 极简 LLM 客户端库
  - 抽象 `Provider` 接口
  - 先支持 DeepSeek + OpenRouter，后面方便扩展
  - 目标：一个包，多 Provider，统一调用方式
- 产出：`llmg` v0.1，能调通两个 Provider 的 chat / stream

### Week 3-4 — 第一个 Agent Loop
- ReAct 论文 + 建 loop：`Observe → Think → Act → Observe …`
- Go 里实现：
  - 流式 SSE 解析
  - 工具调度器
  - 会话历史截断策略
- 把 `llmg` 升级成 `mini-opencode`：
  - 支持注册 tools
  - 多轮循环
  - 命令 `/help`、`/clear`
  - 本地文件读写工具
  - Bash 执行工具（带基本沙箱）
- 仿真 opencode 的核心交互
- **M1 产出**：`mini-opencode` v0.1
  - 能用 DeepSeek 跑通一个能改文件的 CLI agent
  - 对自己的 repo 做简单任务

---

## Month 2：RAG + 知识库

### Week 5 — Embedding & 向量检索原理
- Embedding 模型选型（bge-m3 / embedding-3 / nomic）
- 向量距离、ANN 索引（HNSW）
- 自部署一套 Qdrant（Docker），用 Go 客户端灌入测试数据
- 产出：能向 Qdrant 灌入 + 查询一批向量

### Week 6 — Chunking & 检索质量
- 固定 chunk vs 语义 chunk vs 层级 chunk
- Hybrid search（BM25 + 向量）
- Rerank（cross-encoder / Cohere）
- 评测指标：Recall@k、MRR、人工 spot-check
- **Go 项目**：`raggo` —— RAG 框架库
  - Document Loader / Splitter / Retriever / Reranker 抽象
  - 先支持 markdown / txt / pdf
- 产出：`raggo` v0.1，能对一批文档做检索 + rerank

### Week 7-8 — 知识库 Agent
- 把 `raggo` 接入 `mini-opencode`，加一个 `search_knowledge` 工具
- 处理重点：
  - 引用回溯（让模型回引用 chunk id）
  - 多跳问题
  - 对话式追问
- 数据集：拿你自己的笔记 / 公开文档（如 Go 官方文档）当语料
- **M2 产出**：能用自然语言查 Go 源码或你私人笔记，带证据链

---

## Month 3：Agent 编排 + MCP + 可观测

### Week 9 — 多 Agent 与 Plan 执行
- Plan-and-Execute、LLMCompiler、ReWOO 等 agent 范式对比
- 子 agent 拆分：Planner / Researcher / Coder / Reviewer
- 状态机 vs 自由循环的取舍
- 产出：`mini-opencode` 支持子 agent 调用

### Week 10 — MCP 协议
- 精读 MCP spec（modelcontextprotocol.io）
- 用 Go 写一个 MCP server（参考 `mark3labs/mcp-go`）
- 把 Month 1-2 的工具发布成 MCP server，让 opencode / Claude Desktop 都能用
- **Go 项目**：`mcp-go-tools` —— 通用 MCP 工具集合
  - 文件浏览、git、sqlite 查询等
  - 目标：发布到社区
- 产出：`mcp-go-tools` v0.1，可在 opencode 里直接加载

### Week 11 — 可观测性与 Memory
- Trace：Langfuse / Arize Phoenix 自部署
- Cost / token / 延迟监控
- Agent memory：
  - 短期（会话窗口管理）
  - 长期（向量库存历史，按语义召回）
- 接入到 `mini-opencode`，让 agent 能"记住"你的偏好
- 产出：`mini-opencode` 带 trace 面板和长期记忆

### Week 12 — 评测 + 收尾
- 给 agent 建一个简单 eval 集（SWE-bench 风格的 mini 任务）
- 测量：成功率、平均步数、token 消耗
- 写 README、整理代码、发 GitHub
- 写一篇博客「从零写一个 Go 版 opencode」
- **M3 产出**：可演示、可开源的多工具 Agent 平台 + MCP 工具集 + 可观测面板

---

## Month 4+（可选深挖）
- 多模态（接 Vision，做截图分析工具）
- 本地模型部署（Ollama / vLLM）+ 把你的 agent 接到本地 7B 模型，省成本
- 微调一个小模型专门做 tool 选择（上升到算法侧）
- 接入流式前端（Tauri / Wails），做成桌面 App

---

## 推荐技术栈（Go 生态为主）

| 用途 | 选型 |
|---|---|
| LLM Provider | DeepSeek（成本低）+ OpenRouter（模型多）+ Ollama（本地兜底） |
| Go LLM 库 | 自己写 `llmg` 起步，可参考 `tmc/langchaingo`、`sashabaranov/go-openai` |
| 向量库 | Qdrant（Go client 官方）+ 后期切换 Milvus 测对比 |
| Trace | Langfuse 自部署 |
| Agent 通信 | MCP over stdio + 可选 SSE |
| 部署 | Docker Compose 一把梭 |

---

## 每日 4h 时间分配建议

- **1h**：阅读 / 视频（论文 / 博客 / 文档）
- **2h**：写代码 / debug 项目
- **1h**：复盘 + 写笔记（不写笔记等于没学，强烈建议维护一个学习仓库）

---

## 仓库目录规划建议

```
LLM-Learn/
├── README.md                  # 仓库说明
├── LEARNING_PLAN.md           # 本文件
├── notes/                     # 学习笔记（按周）
│   ├── week-01.md
│   └── ...
├── 01-llmg/                   # Month 1 项目
├── 02-mini-opencode/
├── 03-raggo/                  # Month 2 项目
├── 04-mcp-go-tools/           # Month 3 项目
└── docs/                      # 收集的好文章 / 论文笔记
```

---

## 参考资源清单

### 理论
- Karpathy「Neural Networks: Zero to Hero」：https://karpathy.ai/zero-to-hero.html
- 3Blue1Brown Transformer 系列
- Anthropic Prompt Engineering Guide
- DeepSeek-V3 system prompt（GitHub 开源）

### 协议 / 规范
- MCP spec：https://modelcontextprotocol.io
- OpenAI Function Calling 文档
- DeepSeek API 文档

### Go 生态参考
- `tmc/langchaingo`
- `sashabaranov/go-openai`
- `mark3labs/mcp-go`
- `qdrant/go-client`