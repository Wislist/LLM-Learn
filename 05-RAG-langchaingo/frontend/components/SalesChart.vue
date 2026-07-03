<template>
  <div class="chart">
    <div class="chart__title">{{ spec.title }}</div>
    <client-only>
      <v-chart class="chart__canvas" :option="option" autoresize />
    </client-only>
    <details class="chart__sql"><summary>SQL</summary><pre>{{ spec.sql }}</pre></details>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { BarChart, LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, TitleComponent, LegendComponent, DataZoomComponent } from 'echarts/components'
import VChart from 'vue-echarts'

use([CanvasRenderer, BarChart, LineChart, GridComponent, TooltipComponent, TitleComponent, LegendComponent, DataZoomComponent])

interface ChartPoint { label: string; value: number }
interface ChartSpec { title: string; sql: string; series: ChartPoint[]; chart_type: string }
const props = defineProps<{ spec: ChartSpec }>()

const option = computed(() => {
  const labels = props.spec.series.map(p => p.label)
  const values = props.spec.series.map(p => p.value)
  const type = props.spec.chart_type === 'line' ? 'line' : 'bar'
  return {
    grid: { left: 50, right: 24, top: 20, bottom: 50 },
    tooltip: { trigger: 'axis' },
    dataZoom: labels.length > 12 ? [{ type: 'inside' }, { type: 'slider' }] : [],
    xAxis: { type: 'category', data: labels, axisLabel: { color: '#9ca3af', rotate: labels.length > 6 ? 30 : 0 } },
    yAxis: { type: 'value', axisLabel: { color: '#9ca3af' } },
    series: [{ type, data: values, smooth: true, itemStyle: { color: '#3b5bdb' }, lineStyle: { color: '#3b5bdb' } }],
  }
})
</script>

<style scoped>
.chart { margin-top: 12px; padding: 12px; background: #0f131a; border: 1px solid #232836; border-radius: 8px; }
.chart__title { font-size: 13px; color: #d1d5db; margin-bottom: 6px; }
.chart__canvas { height: 260px; width: 100%; }
.chart__sql { margin-top: 6px; font-size: 11px; color: #6b7280; }
.chart__sql pre { white-space: pre-wrap; word-break: break-all; background: #0a0c10; padding: 8px; border-radius: 6px; }
</style>
