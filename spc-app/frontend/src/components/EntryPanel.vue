<template>
  <div class="card entry-card">
    <h3>录入测量值</h3>
    <p class="muted">
      子组容量 <strong>{{ target.subgroupSize }}</strong>。
      可逐个录入单值，或粘贴一整列（按换行/空格/逗号自动切分），数量须为容量的整数倍。
    </p>

    <div class="entry-row">
      <input
        v-model="single"
        type="text"
        inputmode="decimal"
        :placeholder="`录入单个值（残段 ${chart.incomplete.length}/${target.subgroupSize}）`"
        @keydown.enter="appendSingle"
      />
      <button class="btn btn-primary" @click="appendSingle">追加单值</button>
    </div>

    <textarea
      v-model="columnText"
      rows="5"
      placeholder="粘贴一列，例如&#10;20.01&#10;20.02&#10;20.00&#10;..."
    ></textarea>
    <div class="entry-row">
      <input v-model="operator" type="text" placeholder="检验员（可选）" class="op-input" />
      <button class="btn btn-primary" @click="appendColumn">按整列提交</button>
    </div>

    <p v-if="previewError" class="error">{{ previewError }}</p>
    <p v-else-if="columnTokens.length" class="muted">
      已解析 {{ columnTokens.length }} 个数；
      <span :class="columnAligned ? 'ok' : 'warn'">
        {{ columnAligned
          ? `= ${columnTokens.length / target.subgroupSize} 个完整子组`
          : `不是 ${target.subgroupSize} 的整数倍（将被后端拒绝）` }}
      </span>
    </p>

    <div v-if="result" class="entry-result ok">
      已录入 {{ result.appendedCount }} 个值，新完成 {{ result.newCompletedGroups }} 个子组。
      <span v-if="result.incompleteCount">当前残段 {{ result.incompleteCount }} 个。</span>
    </div>
    <p v-if="serverError" class="error">{{ serverError }}</p>
    <p v-for="(f, i) in fieldErrors" :key="i" class="error field-error">
      {{ f.message }}
    </p>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'
import { api } from '../api/client'

const props = defineProps({
  target: { type: Object, required: true },
  chart: { type: Object, required: true }
})
const emit = defineEmits(['appended'])

const single = ref('')
const columnText = ref('')
const operator = ref('')
const previewError = ref('')
const serverError = ref('')
const fieldErrors = ref([])
const result = ref(null)

function tokenize(text) {
  return text.split(/[\s,，;；]+/).map(s => s.trim()).filter(Boolean)
}

const columnTokens = computed(() => tokenize(columnText.value))
const columnAligned = computed(() =>
  columnTokens.value.length > 0 &&
  columnTokens.value.length % props.target.subgroupSize === 0 &&
  columnTokens.value.every(t => t !== '' && !Number.isNaN(Number(t))))

async function submit(values, source) {
  serverError.value = ''
  fieldErrors.value = []
  result.value = null
  try {
    const res = await api.appendMeasurements(props.target.id, {
      values, operator: operator.value || source
    })
    result.value = res
    emit('appended', res.chart)
  } catch (e) {
    fieldErrors.value = e.data?.fields || []
    serverError.value = fieldErrors.value.length ? '' : e.message
  }
}

async function appendSingle() {
  const v = single.value.trim()
  if (!v) return
  if (Number.isNaN(Number(v))) {
    previewError.value = `“${v}” 不是数字`
    return
  }
  previewError.value = ''
  single.value = ''
  await submit([Number(v)], 'single')
}

async function appendColumn() {
  previewError.value = ''
  const tokens = columnTokens.value
  if (!tokens.length) {
    previewError.value = '请先粘贴测量值'
    return
  }
  const bad = tokens.find(t => Number.isNaN(Number(t)))
  if (bad !== undefined) {
    const pos = tokens.indexOf(bad) + 1
    previewError.value = `第 ${pos} 个测量值不是数字：“${bad}”`
    return
  }
  if (tokens.length % props.target.subgroupSize !== 0) {
    previewError.value = `共 ${tokens.length} 个数，不是子组容量 ${props.target.subgroupSize} 的整数倍`
    return
  }
  columnText.value = ''
  await submit(tokens.map(Number), 'column')
}
</script>
