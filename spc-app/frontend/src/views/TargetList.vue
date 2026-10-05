<template>
  <section>
    <div class="section-head">
      <h2>监控对象</h2>
      <button class="btn" @click="showForm = !showForm">
        {{ showForm ? '取消' : '新建监控对象' }}
      </button>
    </div>

    <form v-if="showForm" class="card form-grid" @submit.prevent="submit">
      <label>名称
        <input v-model="form.name" placeholder="如：1号机床-主轴外径" />
      </label>
      <label>机床
        <input v-model="form.machine" placeholder="如：CNC-01" />
      </label>
      <label>尺寸
        <input v-model="form.dimension" placeholder="如：φ20 外径" />
      </label>
      <label>子组容量 (2–10)
        <input v-model.number="form.subgroupSize" type="number" min="2" max="10" />
      </label>
      <label>规格上限 USL（单侧可留空）
        <input v-model="uslText" placeholder="如：20.05" />
      </label>
      <label>规格下限 LSL（单侧可留空）
        <input v-model="lslText" placeholder="如：19.95" />
      </label>

      <div class="rules-box">
        <span class="rules-title">启用判异规则</span>
        <label v-for="r in RULES" :key="r.id" class="check">
          <input type="checkbox" v-model="form.ruleList" :value="r.id" />
          <span>{{ r.id }} · {{ r.name }}</span>
        </label>
      </div>

      <p v-if="formError" class="error">{{ formError }}</p>
      <p v-for="(f, i) in fieldErrors" :key="i" class="error field-error">
        {{ fieldLabel(f.field) }}：{{ f.message }}
      </p>

      <div>
        <button class="btn btn-primary" type="submit">创建</button>
      </div>
    </form>

    <table v-if="targets.length" class="card table">
      <thead>
        <tr>
          <th>ID</th><th>名称</th><th>机床</th><th>尺寸</th>
          <th>子组容量</th><th>规格 (LSL / USL)</th><th></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="t in targets" :key="t.id">
          <td>{{ t.id }}</td>
          <td>{{ t.name }}</td>
          <td>{{ t.machine }}</td>
          <td>{{ t.dimension }}</td>
          <td>{{ t.subgroupSize }}</td>
          <td>{{ fmt(t.lsl) }} / {{ fmt(t.usl) }}</td>
          <td>
            <router-link class="btn btn-small" :to="`/targets/${t.id}`">打开</router-link>
          </td>
        </tr>
      </tbody>
    </table>
    <p v-else-if="loaded" class="muted">还没有监控对象，点右上角新建。</p>
  </section>
</template>

<script setup>
import { reactive, ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { api, RULES } from '../api/client'

const router = useRouter()
const targets = ref([])
const loaded = ref(false)
const showForm = ref(false)
const formError = ref('')
const fieldErrors = ref([])
const uslText = ref('')
const lslText = ref('')
const form = reactive({
  name: '', machine: '', dimension: '', subgroupSize: 5,
  ruleList: RULES.map(r => r.id)
})

async function load() {
  const data = await api.listTargets()
  targets.value = data.targets || []
  loaded.value = true
}
onMounted(load)

function fmt(v) {
  return v === null || v === undefined ? '—' : v
}
function fieldLabel(f) {
  return { name: '名称', subgroupSize: '子组容量', usl: '规格上限', lsl: '规格下限',
    values: '测量值', rules: '判异规则',
    basisStart: '基准范围', basisEnd: '基准范围' }[f] || f
}

function parseOpt(s) {
  const t = String(s).trim()
  return t === '' ? null : Number(t)
}

async function submit() {
  formError.value = ''
  fieldErrors.value = []
  const usl = parseOpt(uslText.value)
  const lsl = parseOpt(lslText.value)
  const rules = {}
  RULES.forEach(r => { rules[r.id] = form.ruleList.includes(r.id) })
  try {
    const t = await api.createTarget({
      name: form.name, machine: form.machine, dimension: form.dimension,
      subgroupSize: Number(form.subgroupSize), usl, lsl, rules
    })
    router.push(`/targets/${t.id}`)
  } catch (e) {
    fieldErrors.value = e.data?.fields || []
    formError.value = fieldErrors.value.length ? '' : e.message
  }
}
</script>
