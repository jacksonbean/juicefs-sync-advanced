import { useEffect, useState } from 'react'
import { Card, CardContent } from '@/components/ui/card'; import { Button } from '@/components/ui/button'; import { api } from '@/lib/api'; import type { FailedObject } from '@/types'
import { AlertTriangle, Download, RefreshCw, CheckCircle } from 'lucide-react'
export default function Failed() {
  const [data, setData] = useState<FailedObject[]>([])
  useEffect(() => { api.failed.list().then(setData).catch(() => {}) }, [])
  const downloadCsv = () => {
    const headers = ['source_id', 'target_id', 'size', 'error_message', 'attempted_at', 'retry_count']
    const rows = data.map(r => headers.map(h => JSON.stringify((r as any)[h] ?? '')).join(',')); const csv = [headers.join(','), ...rows].join('\n')
    const blob = new Blob([csv], { type: 'text/csv' }); const url = URL.createObjectURL(blob); const a = document.createElement('a'); a.href = url; a.download = 'failed_objects.csv'; a.click(); URL.revokeObjectURL(url)
  }
  if (data.length === 0) return <Card><CardContent className="py-8 text-center"><CheckCircle className="w-12 h-12 mx-auto mb-3 text-emerald-400" /><p className="text-zinc-500">没有失败的同步任务!</p></CardContent></Card>
  return <div className="space-y-6 animate-[slide-up_0.3s_ease-out]"><div className="flex items-center justify-between"><h2 className="text-lg font-bold flex items-center gap-2"><AlertTriangle className="w-5 h-5 text-red-400" />失败的同步</h2><div className="flex gap-2"><Button variant="outline" onClick={downloadCsv} className="gap-2"><Download className="w-4 h-4" />下载 CSV</Button><Button className="gap-2"><RefreshCw className="w-4 h-4" />重试失败的文件</Button></div></div>
    <div className="overflow-x-auto rounded-2xl border border-white/[0.06] bg-white/[0.02] backdrop-blur-xl"><table className="glass-table w-full text-sm"><thead><tr><th>源文件</th><th>目标文件</th><th className="text-right">大小</th><th>错误信息</th><th className="text-left">尝试时间</th><th className="text-right">重试</th></tr></thead><tbody>{data.map((o, i) => <tr key={i} className="border-t border-white/[0.04] hover:bg-white/[0.03] transition-colors"><td className="max-w-[200px] truncate">{o.source_id}</td><td className="max-w-[200px] truncate text-zinc-500">{o.target_id || '-'}</td><td className="text-right font-mono text-xs">{formatBytes(o.size)}</td><td className="max-w-[300px] truncate text-red-400/80 font-mono text-xs">{o.error_message || '-'}</td><td className="text-xs text-zinc-500">{o.attempted_at ? new Date(o.attempted_at).toLocaleString() : '-'}</td><td className="text-right font-mono text-xs">{o.retry_count}</td></tr>)}</tbody></table></div></div>
  function formatBytes(bytes: number): string { if (!bytes) return '0 B'; const units = ['B', 'KB', 'MB', 'GB', 'TB']; let i = 0, size = bytes; while (size >= 1024 && i < units.length - 1) { size /= 1024; i++ }; return `${size.toFixed(2)} ${units[i]}` }
}
