const API_BASE = ''
async function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${url}`, { headers: { 'Content-Type': 'application/json' }, ...init })
  if (!res.ok) { const err = await res.json().catch(() => ({ error: res.statusText })); throw new Error(err.error || res.statusText) }
  return res.json()
}
async function fetchText(url: string): Promise<string> { const res = await fetch(`${API_BASE}${url}`); if (!res.ok) throw new Error(res.statusText); return res.text() }
import type { InstanceInfo, RunSummary, RunDetail, FailedObject, Template, ScheduleTask, ScheduleHistory, AggregatedMetrics } from '../types'
export const api = {
  instances: { list: () => fetchJSON<InstanceInfo[]>('/api/instances') },
  metrics: { aggregated: () => fetchJSON<AggregatedMetrics>('/api/metrics/aggregated'), proxy: (port: number) => fetchText(`/api/metrics?port=${port}`) },
  history: { list: () => fetchJSON<RunSummary[]>('/api/history'), detail: (runId: string) => fetchJSON<RunDetail>(`/api/history/${runId}`), errorsCsv: async (runId: string) => { const res = await fetch(`/api/history/${runId}/errors.csv`); if (!res.ok) throw new Error(res.statusText); return res.text() } },
  failed: { list: () => fetchJSON<FailedObject[]>('/api/failed') },
  templates: { list: () => fetchJSON<Template[]>('/api/templates'), create: (t: Omit<Template, 'id' | 'created_at' | 'updated_at'>) => fetchJSON<{ status: string }>('/api/templates', { method: 'POST', body: JSON.stringify(t) }), update: (id: number, t: Partial<Template>) => fetchJSON<{ status: string }>(`/api/templates/${id}`, { method: 'PUT', body: JSON.stringify(t) }), delete: (id: number) => fetchJSON<{ status: string }>(`/api/templates/${id}`, { method: 'DELETE' }) },
  schedules: { list: () => fetchJSON<ScheduleTask[]>('/api/schedules'), create: (t: Omit<ScheduleTask, 'id' | 'created_at' | 'last_run' | 'next_run'>) => fetchJSON<{ status: string }>('/api/schedules', { method: 'POST', body: JSON.stringify(t) }), update: (id: number, t: Partial<ScheduleTask>) => fetchJSON<{ status: string }>(`/api/schedules/${id}`, { method: 'PUT', body: JSON.stringify(t) }), delete: (id: number) => fetchJSON<{ status: string }>(`/api/schedules/${id}`, { method: 'DELETE' }), history: (taskId: number) => fetchJSON<ScheduleHistory[]>(`/api/schedules/${taskId}/history`) },
  sync: { start: (req: { src: string; dst: string; threads?: number; dry_run?: boolean; delete_dst?: boolean; update?: boolean }) => fetchJSON<{ command: string; message: string }>('/api/sync', { method: 'POST', body: JSON.stringify(req) }) },
}
