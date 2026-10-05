// 与后端的唯一通信入口。前端不计算任何控制限/能力指数，只展示这些数字。

const BASE = '/api'

async function request(method, path, body) {
  const res = await fetch(BASE + path, {
    method,
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined
  })
  let data = null
  const text = await res.text()
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = { message: text }
    }
  }
  if (!res.ok) {
    const err = new Error(data?.message || `请求失败 (${res.status})`)
    err.status = res.status
    err.data = data
    throw err
  }
  return data
}

export const api = {
  listTargets: () => request('GET', '/targets'),
  createTarget: (input) => request('POST', '/targets', input),
  getTarget: (id) => request('GET', `/targets/${id}`),
  updateTarget: (id, input) => request('PUT', `/targets/${id}`, input),
  getChart: (id) => request('GET', `/targets/${id}/chart`),
  appendMeasurements: (id, payload) => request('POST', `/targets/${id}/measurements`, payload),
  rebaseline: (id, payload) => request('POST', `/targets/${id}/rebaseline`, payload || {})
}

export const RULES = [
  { id: 'R1', name: '一点超出三倍标准差' },
  { id: 'R2', name: '连续九点落在中心线同侧' },
  { id: 'R3', name: '连续六点递增或递减' },
  { id: 'R4', name: '三点中有两点落在同侧两倍到三倍之间' }
]
