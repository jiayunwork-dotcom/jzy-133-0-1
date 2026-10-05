<template>
  <div class="chart-wrap">
    <div class="chart-head">
      <span class="chart-title">{{ title }}（{{ chart.subgroups.length }} 组）</span>
      <span v-if="activeBaseline" class="muted">
        当前基准 v{{ activeBaseline.version }}：第 {{ activeBaseline.startGroup }} 组起
      </span>
    </div>
    <svg v-if="points.length" :viewBox="`0 0 ${width} ${height}`" class="chart-svg">
      <!-- 网格与坐标 -->
      <line v-for="(t, i) in yTicks" :key="'g'+i"
            :x1="pad.l" :x2="width-pad.r"
            :y1="y(t)" :y2="y(t)"
            stroke="#eef1f5" stroke-width="1" />
      <text v-for="(t, i) in yTicks" :key="'t'+i"
            :x="pad.l-6" :y="y(t)+3.5" text-anchor="end" class="axis-label">
        {{ t.toFixed(decimals) }}
      </text>

      <!-- 每个基准段：限 + 中心线（线段，区间为该限的生效范围）-->
      <template v-for="seg in segments" :key="seg.key">
        <line :x1="seg.x1" :x2="seg.x2" :y1="y(seg.cl)" :y2="y(seg.cl)"
              :stroke="seg.color" stroke-width="1.6" stroke-dasharray="6 4" />
        <line :x1="seg.x1" :x2="seg.x2" :y1="y(seg.ucl)" :y2="y(seg.ucl)"
              :stroke="seg.color" stroke-width="1.2" />
        <line :x1="seg.x1" :x2="seg.x2" :y1="y(seg.lcl)" :y2="y(seg.lcl)"
              :stroke="seg.color" stroke-width="1.2" />
        <text :x="seg.x2 - 4" :y="y(seg.ucl)-4" :fill="seg.color"
              text-anchor="end" class="lim-label">UCL v{{ seg.version }}</text>
        <text :x="seg.x2 - 4" :y="y(seg.cl)-4" :fill="seg.color"
              text-anchor="end" class="lim-label">CL v{{ seg.version }}</text>
      </template>

      <!-- 数据折线 -->
      <polyline :points="linePoints" fill="none" stroke="#375a7f" stroke-width="1.6" />

      <!-- 数据点：命中规则的点按告警色 -->
      <g v-for="(p, i) in drawPoints" :key="'p'+i">
        <circle :cx="p.x" :cy="p.y" :r="p.alarm ? 5 : 3.2"
                :fill="p.alarm ? alarmColor(p.rules) : '#375a7f'"
                :stroke="p.alarm ? '#7a1f1f' : '#fff'" stroke-width="1">
          <title>子组 {{ p.index }}：{{ p.value.toFixed(4) }}
            <template v-if="p.rules.length">（{{ p.rules.join(', ') }}）</template>
          </title>
        </circle>
        <text :x="p.x" :y="height-8" text-anchor="middle" class="axis-label">{{ p.index }}</text>
      </g>
    </svg>
    <p v-else class="muted chart-empty">尚无完整子组，先录入至少一个子组的数据。</p>
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  chart: { type: Object, required: true },
  metric: { type: String, default: 'mean' }, // mean | range
  title: { type: String, default: '均值图 Xbar' }
})

const width = 920
const height = 320
const pad = { l: 56, r: 20, t: 18, b: 26 }

const activeBaseline = computed(() =>
  props.chart.baselines.find(b => b.active) || null)

const points = computed(() => props.chart.subgroups)

// 选值与限：均值图用 mean + xbar 限，极差图用 range + r 限。
const drawPoints = computed(() => {
  return points.value.map(s => {
    const value = props.metric === 'mean' ? s.mean : s.range
    return {
      index: s.index,
      value,
      alarm: (props.chart.alarmIndices[s.index] || []).length > 0,
      rules: props.metric === 'mean' ? (props.chart.alarmIndices[s.index] || []) : []
    }
  })
})

const segValue = (b) => props.metric === 'mean'
  ? { cl: b.xbarCl, ucl: b.xbarU, lcl: b.xbarL }
  : { cl: b.rbarCl, ucl: b.rbarU, lcl: b.rbarL }

const yMin = computed(() => {
  let lo = Infinity
  const consider = (v) => { if (Number.isFinite(v) && v < lo) lo = v }
  drawPoints.value.forEach(p => consider(p.value))
  props.chart.baselines.forEach(b => {
    const s = segValue(b)
    consider(s.lcl); consider(s.ucl); consider(s.cl)
  })
  if (!Number.isFinite(lo)) lo = 0
  return lo
})
const yMax = computed(() => {
  let hi = -Infinity
  const consider = (v) => { if (Number.isFinite(v) && v > hi) hi = v }
  drawPoints.value.forEach(p => consider(p.value))
  props.chart.baselines.forEach(b => {
    const s = segValue(b)
    consider(s.lcl); consider(s.ucl); consider(s.cl)
  })
  if (!Number.isFinite(hi)) hi = 1
  if (hi === yMin.value) hi = yMin.value + 1
  const padV = (hi - yMin.value) * 0.08
  return hi + padV
})

const decimals = computed(() => {
  const span = yMax.value - yMin.value
  return span < 0.01 ? 4 : span < 0.5 ? 3 : 2
})

function y(v) {
  const usable = height - pad.t - pad.b
  return pad.t + (1 - (v - yMin.value) / (yMax.value - yMin.value)) * usable
}
function x(index) {
  const n = Math.max(drawPoints.value.length, 1)
  const usable = width - pad.l - pad.r
  return pad.l + ((index - 1) / Math.max(n - 1, 1)) * usable
}

const linePoints = computed(() =>
  drawPoints.value.map(p => `${x(p.index).toFixed(1)},${y(p.value).toFixed(1)}`).join(' '))

const yTicks = computed(() => {
  const ticks = []
  const N = 5
  for (let i = 0; i <= N; i++) {
    ticks.push(yMin.value + (yMax.value - yMin.value) * i / N)
  }
  return ticks
})

// 为每条基准计算其生效的子组横坐标区间 [start, 下一基准 start-1 或当前末尾]。
const segments = computed(() => {
  const bs = [...props.chart.baselines].sort((a, b2) => a.startGroup - b2.startGroup)
  const lastIndex = drawPoints.value.length
  const palette = ['#2a6f97', '#8a4b1e', '#4f772d', '#6d2e73', '#1d6f6f']
  return bs.map((b, i) => {
    const next = bs[i + 1]
    const startIdx = Math.min(Math.max(b.startGroup, 1), Math.max(lastIndex, 1))
    const endIdx = next ? Math.max(next.startGroup - 1, startIdx) : Math.max(lastIndex, startIdx)
    const s = segValue(b)
    return {
      key: b.id,
      version: b.version,
      color: palette[i % palette.length],
      x1: x(startIdx),
      x2: x(endIdx),
      ...s
    }
  }).filter(seg => seg.x2 >= pad.l)
})

const RULE_COLORS = { R1: '#d62728', R2: '#ff7f0e', R3: '#9467bd', R4: '#e377c2' }
function alarmColor(ruleList) {
  // 一个点命中多条时，取编号最小规则的颜色（最严重的超限优先）。
  const order = ['R1', 'R2', 'R3', 'R4']
  const hit = order.find(r => ruleList.includes(r))
  return RULE_COLORS[hit] || '#d62728'
}
</script>
