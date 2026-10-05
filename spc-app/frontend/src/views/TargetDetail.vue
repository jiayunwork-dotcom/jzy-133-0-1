<template>
  <section v-if="target" class="detail">
    <div class="section-head">
      <div>
        <router-link to="/" class="back">← 监控对象列表</router-link>
        <h2>{{ target.name }} <span class="muted">{{ target.machine }} · {{ target.dimension }}</span></h2>
      </div>
      <div class="head-actions">
        <span v-if="chart.state === 'collecting'" class="badge badge-warn">
          基准采集中（{{ chart.subgroups.length }}/20 组）
        </span>
        <span v-else class="badge badge-ok">已冻结控制限</span>
        <button class="btn btn-primary" :disabled="chart.subgroups.length < 20 || busy"
                @click="rebaseline">
          {{ chart.state === 'frozen' ? '重新基准' : '冻结基准' }}
        </button>
        <button class="btn" @click="showConfig = !showConfig">档案配置</button>
      </div>
    </div>

    <!-- 档案配置（规格/判异开关；子组容量不可改） -->
    <form v-if="showConfig" class="card form-grid" @submit.prevent="saveConfig">
      <label>名称<input v-model="cfg.name" /></label>
      <label>机床<input v-model="cfg.machine" /></label>
      <label>尺寸<input v-model="cfg.dimension" /></label>
      <label>子组容量（不可改）
        <input :value="target.subgroupSize" disabled />
      </label>
      <label>USL<input v-model="cfg.uslText" placeholder="单侧可空" /></label>
      <label>LSL<input v-model="cfg.lslText" placeholder="单侧可空" /></label>
      <div class="rules-box">
        <span class="rules-title">判异规则（改动立即对当前序列重放）</span>
        <label v-for="r in RULES" :key="r.id" class="check">
          <input type="checkbox" v-model="cfg.ruleList" :value="r.id" />
          <span>{{ r.id }} · {{ r.name }}</span>
        </label>
      </div>
      <p v-for="(f, i) in cfgErrors" :key="i" class="error field-error">{{ f.message }}</p>
      <div><button class="btn btn-primary" type="submit">保存配置</button></div>
    </form>

    <!-- 能力指数（只来自后端，按基准期） -->
    <div v-if="activeBaseline" class="card capability">
      <div class="cap-item">
        <span class="cap-label">Cp</span>
        <span class="cap-value">{{ fmt(activeBaseline.cp) }}</span>
      </div>
      <div class="cap-item">
        <span class="cap-label">Cpk</span>
        <span class="cap-value">{{ fmt(activeBaseline.cpk) }}</span>
      </div>
      <div class="cap-item">
        <span class="cap-label">组内 σ（Rbar/d2）</span>
        <span class="cap-value">{{ activeBaseline.sigma.toFixed(5) }}</span>
      </div>
      <div class="cap-item">
        <span class="cap-label">基准子组数</span>
        <span class="cap-value">{{ activeBaseline.subgroupCount }}</span>
      </div>
      <p v-if="activeBaseline.note" class="warn cap-note">⚠ {{ activeBaseline.note }}</p>
    </div>

    <!-- 主图 -->
    <ControlChart :chart="chart" metric="mean" title="均值图 Xbar" />
    <ControlChart :chart="chart" metric="range" title="极差图 R" />

    <div class="two-col">
      <EntryPanel :target="target" :chart="chart" @appended="onAppended" />

      <div class="card alarm-card">
        <h3>判异告警（{{ chart.alarms.length }}）</h3>
        <p v-if="!chart.alarms.length" class="muted">暂无告警。</p>
        <ul v-else class="alarm-list">
          <li v-for="a in alarmGroups" :key="a.version+'-'+a.rule+a.firstIndex+a.index">
            <span :class="'tag tag-'+a.rule">{{ a.rule }}</span>
            <span class="muted">v{{ a.version }}</span>
            <span>{{ a.message }}</span>
          </li>
        </ul>
      </div>
    </div>
  </section>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { api, RULES } from '../api/client'
import ControlChart from '../components/ControlChart.vue'
import EntryPanel from '../components/EntryPanel.vue'

const props = defineProps({ id: [String, Number] })

const target = ref(null)
const chart = ref({ target: null, state: 'collecting', subgroups: [], incomplete: [],
  baselines: [], alarms: [], alarmIndices: {}, totalMeasurements: 0 })
const showConfig = ref(false)
const busy = ref(false)
const cfgErrors = ref([])
const cfg = reactive({ name: '', machine: '', dimension: '', uslText: '', lslText: '',
  ruleList: [] })

const activeBaseline = computed(() => chart.value.baselines.find(b => b.active) || null)
// 全部版本的告警都留档展示（旧版本告警置灰，当前版本正常高亮）。
const alarmGroups = computed(() => chart.value.alarms)

async function load() {
  target.value = await api.getTarget(props.id)
  chart.value = await api.getChart(props.id)
  Object.assign(cfg, {
    name: target.value.name,
    machine: target.value.machine,
    dimension: target.value.dimension,
    uslText: target.value.usl ?? '',
    lslText: target.value.lsl ?? '',
    ruleList: RULES.filter(r => target.value.rules[r.id]).map(r => r.id)
  })
}
onMounted(load)

function onAppended(nextChart) {
  chart.value = nextChart
}
function fmt(v) {
  return v === null || v === undefined ? '—' : Number(v).toFixed(3)
}

async function rebaseline() {
  if (chart.value.subgroups.length < 20) return
  busy.value = true
  try {
    await api.rebaseline(props.id, {})
    await load()
  } finally {
    busy.value = false
  }
}

async function saveConfig() {
  cfgErrors.value = []
  const parseOpt = s => String(s).trim() === '' ? null : Number(s)
  const rules = {}
  RULES.forEach(r => { rules[r.id] = cfg.ruleList.includes(r.id) })
  try {
    const updated = await api.updateTarget(props.id, {
      name: cfg.name, machine: cfg.machine, dimension: cfg.dimension,
      subgroupSize: target.value.subgroupSize,
      usl: parseOpt(cfg.uslText), lsl: parseOpt(cfg.lslText), rules
    })
    target.value = updated
    chart.value = await api.getChart(props.id)
    showConfig.value = false
  } catch (e) {
    cfgErrors.value = e.data?.fields || [{ message: e.message }]
  }
}
</script>
