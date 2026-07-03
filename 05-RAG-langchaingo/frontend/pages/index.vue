<template>
  <div class="app">
    <header class="app__header">
      <h1>RAG 知识库 <span class="muted">· 报价预测 / 销量分析</span></h1>
      <div class="app__status">
        <span :class="['dot', health.ollama === 'ok' ? 'ok' : 'err']" /> Ollama
        <span :class="['dot', health.milvus === 'ok' ? 'ok' : 'err']" /> Milvus
      </div>
    </header>

    <main class="app__main">
      <aside class="sidebar">
        <FileUpload @uploaded="onUploaded" />
        <div class="hint">
          <p>支持 .docx / .pdf / .xlsx</p>
          <p>Excel 会按表头自动分流：销量明细入库 DuckDB，报价单入库向量库。</p>
        </div>
      </aside>

      <section class="chat">
        <div class="chat__scroll" ref="scrollEl">
          <div v-for="m in messages" :key="m.id" :class="['msg', m.role]">
            <div class="msg__role">{{ m.role === 'user' ? '我' : 'AI' }}</div>
            <div class="msg__body">
              <div class="msg__text" v-html="renderMarkdown(m.text)"></div>
              <SalesChart v-if="m.chart" :spec="m.chart" />
              <div class="msg__ctx" v-if="m.context && m.context.length">
                <details>
                  <summary>检索上下文 ({{ m.context.length }})</summary>
                  <div v-for="(c, i) in m.context" :key="i" class="ctx">
                    <span class="ctx__score">{{ (c.score).toFixed(3) }}</span>
                    <span class="ctx__src">{{ c.source }}</span>
                    <span class="ctx__text">{{ c.content || fieldText(c) }}</span>
                  </div>
                </details>
              </div>
            </div>
          </div>
          <div v-if="loading" class="msg assistant">
            <div class="msg__role">AI</div>
            <div class="msg__body"><span class="typing">思考中…</span></div>
          </div>
        </div>

        <form class="chat__input" @submit.prevent="send">
          <textarea
            v-model="input"
            placeholder="问：给某地铁通风系统项目报价 / 近 12 个月各产品销量趋势"
            @keydown.enter.exact.prevent="send"
            rows="2"
          />
          <button type="submit" :disabled="loading || !input.trim()">发送</button>
        </form>
      </section>
    </main>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, nextTick } from 'vue'

interface ChartPoint { label: string; value: number }
interface ChartSpec { title: string; sql: string; series: ChartPoint[]; chart_type: string }
interface SearchHit { score: number; content: string; source: string; fields?: Record<string, string> }
interface Answer { text: string; context?: SearchHit[]; chart?: ChartSpec; route: string }
interface Message {
  id: number
  role: 'user' | 'assistant'
  text: string
  context?: SearchHit[]
  chart?: ChartSpec
}

const config = useRuntimeConfig()
const apiBase = config.public.apiBase as string

const input = ref('')
const messages = reactive<Message[]>([])
const loading = ref(false)
const scrollEl = ref<HTMLElement | null>(null)
const health = reactive<{ ollama: string; milvus: string }>({ ollama: '?', milvus: '?' })
let seq = 0

onMounted(async () => {
  try {
    const r = await fetch(`${apiBase}/healthz`)
    const j = await r.json()
    health.ollama = j.ollama || '?'
    health.milvus = j.milvus || '?'
  } catch {
    health.ollama = 'err'
    health.milvus = 'err'
  }
})

function fieldText(c: SearchHit): string {
  if (!c.fields) return ''
  return Object.entries(c.fields).map(([k, v]) => `${k}=${v}`).join(' | ')
}

function renderMarkdown(s: string): string {
  // tiny markdown: bold + line breaks. Avoid pulling a full lib for MVP.
  return (s || '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
    .replace(/\n/g, '<br>')
}

function onUploaded(info: { kind: string; chunks?: number; rows?: number; error?: string }) {
  messages.push({
    id: ++seq,
    role: 'assistant',
    text: info.error
      ? `上传失败: ${info.error}`
      : `已入库（${info.kind}，${info.chunks ?? info.rows ?? 0} 条）。`,
  })
  scrollBottom()
}

async function send() {
  const q = input.value.trim()
  if (!q || loading.value) return
  input.value = ''
  messages.push({ id: ++seq, role: 'user', text: q })
  const msg: Message = { id: ++seq, role: 'assistant', text: '' }
  messages.push(msg)
  loading.value = true
  scrollBottom()

  try {
    const res = await fetch(`${apiBase}/api/chat`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ question: q, session: 'web' }),
    })
    if (!res.ok || !res.body) {
      msg.text = `请求失败: ${res.status}`
      loading.value = false
      return
    }
    const reader = res.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      const events = buffer.split('\n\n')
      buffer = events.pop() || ''
      for (const ev of events) {
        const lines = ev.split('\n')
        let event = 'message'
        let data = ''
        for (const ln of lines) {
          if (ln.startsWith('event:')) event = ln.slice(6).trim()
          else if (ln.startsWith('data:')) data += ln.slice(5).trim()
        }
        if (!data) continue
        try {
          const payload = JSON.parse(data)
          if (event === 'delta' && payload.text) {
            msg.text += payload.text
            scrollBottom()
          } else if (event === 'done') {
            const ans = payload as Answer
            msg.text = ans.text || msg.text
            msg.context = ans.context
            msg.chart = ans.chart
          } else if (event === 'error') {
            msg.text = `错误: ${payload.message}`
          }
        } catch { /* ignore malformed */ }
      }
    }
  } catch (e: any) {
    msg.text = `网络错误: ${e?.message || e}`
  } finally {
    loading.value = false
    scrollBottom()
  }
}

function scrollBottom() {
  nextTick(() => {
    if (scrollEl.value) scrollEl.value.scrollTop = scrollEl.value.scrollHeight
  })
}
</script>

<style>
* { box-sizing: border-box; }
body { margin: 0; font-family: -apple-system, "PingFang SC", "Microsoft YaHei", sans-serif; background: #0f1115; color: #e6e6e6; }
.app { display: flex; flex-direction: column; height: 100vh; }
.app__header { display: flex; justify-content: space-between; align-items: center; padding: 14px 22px; background: #161922; border-bottom: 1px solid #232836; }
.app__header h1 { font-size: 16px; font-weight: 600; margin: 0; }
.muted { color: #6b7280; font-weight: 400; }
.app__status { font-size: 12px; color: #9ca3af; }
.dot { display: inline-block; width: 8px; height: 8px; border-radius: 50%; margin: 0 4px 0 10px; background: #555; }
.dot.ok { background: #22c55e; } .dot.err { background: #ef4444; }
.app__main { flex: 1; display: grid; grid-template-columns: 280px 1fr; min-height: 0; }
.sidebar { padding: 18px; border-right: 1px solid #232836; background: #12151c; overflow: auto; }
.hint { margin-top: 14px; font-size: 12px; color: #6b7280; line-height: 1.6; }
.hint p { margin: 0 0 6px; }
.chat { display: flex; flex-direction: column; min-height: 0; }
.chat__scroll { flex: 1; overflow-y: auto; padding: 20px 24px; }
.msg { display: flex; gap: 12px; margin-bottom: 18px; }
.msg.user { flex-direction: row-reverse; }
.msg__role { flex: 0 0 28px; height: 28px; border-radius: 50%; background: #2a2f3d; display: flex; align-items: center; justify-content: center; font-size: 12px; }
.msg.user .msg__role { background: #3b5bdb; }
.msg__body { max-width: 78%; padding: 10px 14px; border-radius: 10px; background: #1a1e27; }
.msg.user .msg__body { background: #1e3a8a; }
.msg__text { font-size: 14px; line-height: 1.7; white-space: normal; word-break: break-word; }
.msg__ctx { margin-top: 10px; font-size: 12px; }
.msg__ctx summary { cursor: pointer; color: #6b7280; }
.ctx { display: flex; gap: 8px; padding: 4px 0; color: #9ca3af; border-bottom: 1px dashed #232836; }
.ctx__score { color: #f59e0b; flex: 0 0 50px; }
.ctx__src { color: #60a5fa; flex: 0 0 120px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ctx__text { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.typing { color: #6b7280; font-style: italic; }
.chat__input { display: flex; gap: 10px; padding: 14px 24px 18px; border-top: 1px solid #232836; background: #12151c; }
.chat__input textarea { flex: 1; resize: none; background: #1a1e27; color: #e6e6e6; border: 1px solid #2a2f3d; border-radius: 8px; padding: 10px 12px; font-size: 14px; font-family: inherit; }
.chat__input button { padding: 0 22px; border: none; border-radius: 8px; background: #3b5bdb; color: #fff; font-size: 14px; cursor: pointer; }
.chat__input button:disabled { opacity: .5; cursor: not-allowed; }
</style>
