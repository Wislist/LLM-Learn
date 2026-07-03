<template>
  <div class="upload">
    <label class="upload__drop">
      <input type="file" accept=".docx,.pdf,.xlsx,.xls" @change="onChange" hidden />
      <span v-if="!busy">拖拽或点击上传文件</span>
      <span v-else>处理中… {{ progress }}</span>
    </label>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

const config = useRuntimeConfig()
const apiBase = config.public.apiBase as string
const emit = defineEmits<{ (e: 'uploaded', info: { kind: string; chunks?: number; rows?: number; error?: string }): void }>()

const busy = ref(false)
const progress = ref('')

async function onChange(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  busy.value = true
  progress.value = file.name
  const fd = new FormData()
  fd.append('file', file)
  try {
    const r = await fetch(`${apiBase}/api/upload`, { method: 'POST', body: fd })
    const j = await r.json()
    emit('uploaded', j)
  } catch (err: any) {
    emit('uploaded', { kind: 'error', error: err?.message || String(err) })
  } finally {
    busy.value = false
    input.value = ''
  }
}
</script>

<style scoped>
.upload__drop { display: flex; align-items: center; justify-content: center; height: 90px; border: 1.5px dashed #2a2f3d; border-radius: 10px; cursor: pointer; font-size: 13px; color: #9ca3af; transition: border-color .15s, background .15s; }
.upload__drop:hover { border-color: #3b5bdb; background: #1a1e27; }
</style>
