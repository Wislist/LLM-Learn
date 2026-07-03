export default defineNuxtConfig({
  devServer: { port: 3000, host: '0.0.0.0' },
  app: {
    head: {
      title: 'RAG 知识库 · 报价预测',
      meta: [{ name: 'viewport', content: 'width=device-width, initial-scale=1' }],
    },
  },
  runtimeConfig: {
    public: {
      apiBase: process.env.NUXT_PUBLIC_API_BASE || 'http://localhost:8787',
    },
  },
  build: {
    transpile: ['vue-echarts', 'echarts'],
  },
})
