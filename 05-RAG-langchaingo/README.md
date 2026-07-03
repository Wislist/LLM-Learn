# 05-RAG-langchaingo

基于 RAG 知识库 + AI 自动化的企业业务系统：报价/投标金额预测、Excel 检索、销量图表生成。

## 技术栈

| 层 | 选型 |
|---|---|
| 后端 | Go + [langchaingo](https://github.com/tmc/langchaingo) |
| LLM | DeepSeek Chat（OpenAI 兼容） |
| Embedding | Ollama `bge-m3`（1024 维） |
| 向量库 | Milvus Standalone（docker-compose） |
| 结构化库 | DuckDB（嵌入式，单文件） |
| 前端 | Nuxt 3 + ECharts |

## 架构：Hybrid RAG

不同类型的数据走各自擅长的检索路径，再统一喂给 LLM：

```
用户问题
   ↓ 关键词路由
   ├─ 报价/文档类 → Milvus 向量检索 → DeepSeek 推理
   └─ 销量/图表类 → DeepSeek 生成 SQL → DuckDB 聚合 → ECharts
```

详见 [设计说明](#架构说明)。

## 目录

```
05-RAG-langchaingo/
├── docker-compose.yml          # milvus + etcd + minio + ollama
├── .env.example
├── backend/
│   ├── cmd/server/main.go      # HTTP 服务入口
│   ├── scripts/init_milvus/    # 建表脚本
│   └── internal/
│       ├── config/  llm/  embedder/  milvus/
│       ├── store/   (DuckDB)
│       ├── ingest/  (docx/pdf/excel 解析+切片)
│       ├── rag/     (检索+prompt+流式)
│       └── api/     (HTTP handlers)
└── frontend/                   # Nuxt 3
```

## 快速开始

### 1. 启动基础设施

```bash
docker compose up -d
# 拉取 embedding 模型（首次，约 1.2GB）
docker compose exec ollama ollama pull bge-m3
```

> macOS Apple Silicon：Docker 内 Ollama 无法用 GPU。如需更快，可本机 `brew install ollama && ollama serve`，再在 `.env` 把 `OLLAMA_HOST` 保持 `http://localhost:11434`，并注释掉 compose 里的 ollama 服务。

### 2. 配置

```bash
cp .env.example .env
# 编辑 .env，填入 DEEPSEEK_API_KEY
```

### 3. 后端

```bash
cd backend
go mod tidy
# 首次：建 Milvus collection
go run ./scripts/init_milvus
# 启动服务（:8787）
go run ./cmd/server
```

### 4. 前端

```bash
cd frontend
pnpm install
pnpm dev    # http://localhost:3000
```

## 使用

1. 在网页左侧上传文件：
   - `.docx` / `.pdf` → 切片向量化入 Milvus `documents`
   - `.xlsx`（表头含 `project_name,item_name,unit_price`）→ 报价单入 Milvus `quotes`
   - `.xlsx`（表头含 `order_date,product,qty`）→ 销量明细入 DuckDB `sales`
   - 其他 `.xlsx` → 作为叙述文本入 `documents`
2. 对话框提问：
   - "给某地铁通风系统项目报价" → 检索历史报价 + LLM 给出区间与拆解
   - "近 12 个月各产品销量趋势" → 生成 SQL → 画折线图
   - "各区域销售额对比" → 画柱状图

## 架构说明

### 为什么销量图表不纯走向量检索？

向量检索（Milvus）擅长"找语义相似的文字片段"，不擅长做 `SUM(qty) GROUP BY month` 这类聚合统计：
把 10 万行销量切片向量化，LLM 只能看到 top-k=20 条，算出来的"总销量"必然是错的。

所以销量明细存 DuckDB（列式 OLAP，聚合快），由 LLM 做 **Text-to-SQL** 取数，再回灌 LLM 生成文字摘要，
图表数据直接回前端 ECharts 渲染。这条路径不涉及"Agent 工具调用 / 多步循环"，只是后端流水线里的一个分支，
工程量与纯 RAG 接近。

### Milvus Collections

- `rag_documents`：`id, source, doc_type, content(8192), vector(1024)`
- `rag_quotes`：`id, project_name, item_name, qty, unit_price, total, source, content, vector(1024)`
- 索引：`IVF_FLAT` + `IP`（内积，配合 bge-m3 归一化向量等价于余弦）

### DuckDB 表

```sql
sales(order_date DATE, product VARCHAR, region VARCHAR,
      qty DOUBLE, amount DOUBLE, source VARCHAR)
chat_log(ts TIMESTAMP, role VARCHAR, content VARCHAR, session VARCHAR)
```

## 后续优化方向

- 重排（rerank）：用 bge-reranker 对 top-20 重排后再取 top-6
- 混合路由升级：LLM 意图分类替代关键词
- 多轮对话记忆：把 chat_log 摘要拼进上下文
- Excel 报价单：按"项目"做父文档，行做子文档，检索时父子回填
