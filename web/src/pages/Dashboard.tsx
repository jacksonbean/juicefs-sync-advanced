import { useEffect, useRef, useState } from 'react'
import { api } from '@/lib/api'
import type { InstanceInfo, AggregatedMetrics } from '@/types'
import { useCountUp } from '@/hooks/useCountUp'
import { Activity, HardDrive, Copy, AlertCircle, Network, Cpu, Timer } from 'lucide-react'
import { AreaChart, Area, ResponsiveContainer } from 'recharts'
export default function Dashboard() {
  const [metrics, setMetrics] = useState<AggregatedMetrics | null>(null)
  const [instances, setInstances] = useState<InstanceInfo[]>([])
  const [loading, setLoading] = useState(true)
  useEffect(() => {
    const fetchData = async () => {
      try { const [m, i] = await Promise.all([api.metrics.aggregated(), api.instances.list()]); setMetrics(m); setInstances(i) }
      catch (e) { console.error(e) } finally { setLoading(false) }
    }; fetchData(); const interval = setInterval(fetchData, 3000); return () => clearInterval(interval)
  }, [])
  const stats = metrics || { total: 0, running: 0, scanned: 0, copied: 0, failed: 0 }
  if (loading) return <div className="flex items-center justify-center py-24"><div className="w-8 h-8 border-2 border-indigo-500 border-t-transparent rounded-full animate-spin" /></div>
  return (
    <div className="space-y-8 animate-[slide-up_0.4s_ease-out]">
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <StatCard value={stats.running} label="运行中" icon={Activity} gradient="from-emerald-500/20 to-emerald-600/10 border-emerald-500/20" textGradient="from-emerald-200 to-emerald-400" />
        <StatCard value={stats.total} label="总任务" icon={HardDrive} gradient="from-blue-500/20 to-indigo-600/10 border-blue-500/20" textGradient="from-blue-200 to-indigo-400" />
        <StatCard value={stats.copied} label="已复制" icon={Copy} gradient="from-purple-500/20 to-pink-600/10 border-purple-500/20" textGradient="from-purple-200 to-pink-400" />
        <StatCard value={stats.failed} label="失败" icon={AlertCircle} gradient="from-red-500/20 to-orange-600/10 border-red-500/20" textGradient="from-red-200 to-orange-400" />
      </div>
      {instances.length === 0 ? (
        <div className="rounded-2xl border border-white/[0.06] bg-white/[0.02] backdrop-blur-xl p-16 text-center">
          <div className="flex justify-center mb-4"><div className="w-16 h-16 rounded-full bg-zinc-800/50 flex items-center justify-center"><Activity className="w-8 h-8 text-zinc-500" /></div></div>
          <p className="text-lg text-zinc-400">暂无运行中的同步任务</p>
          <p className="text-sm text-zinc-600 mt-2">切换到「新任务」标签创建同步</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-5">{instances.map((inst, i) => <InstanceCard key={inst.port} inst={inst} index={i} />)}</div>
      )}
    </div>
  )
}
function StatCard({ value, label, icon: Icon, gradient, textGradient }: { value: number; label: string; icon: React.ComponentType<{ className?: string }>; gradient: string; textGradient: string }) {
  const animated = useCountUp(value)
  return <div className={`relative rounded-2xl bg-gradient-to-br ${gradient} border p-6 text-center overflow-hidden group hover:scale-[1.02] transition-all duration-300 glow-pulse`}>
    <div className="absolute inset-0 bg-gradient-to-t from-black/20 to-transparent pointer-events-none" />
    <div className="relative"><Icon className="w-5 h-5 mx-auto mb-3 text-zinc-400 group-hover:text-white transition-colors" />
      <div className={`text-4xl font-bold bg-gradient-to-r ${textGradient} bg-clip-text text-transparent tabular-nums`}>{animated.toLocaleString()}</div>
      <div className="text-xs text-zinc-500 uppercase tracking-widest mt-2 font-medium">{label}</div>
    </div>
  </div>
}
function InstanceCard({ inst, index }: { inst: InstanceInfo; index: number }) {
  const [status, setStatus] = useState<'running' | 'idle' | 'unknown'>('unknown')
  const [scanned, setScanned] = useState(0); const [copied, setCopied] = useState(0); const [failed, setFailed] = useState(0); const [bytesCopied, setBytesCopied] = useState(0)
  const [speedHistory, setSpeedHistory] = useState<{ t: string; v: number }[]>([]); const prevBytes = useRef(0)
  useEffect(() => {
    const fetchMetrics = async () => {
      try {
        const text = await api.metrics.proxy(inst.port); let s = 0, c = 0, f = 0, b = 0
        for (const line of text.split('\n')) {
          if (line.startsWith('#') || !line.trim()) continue; const parts = line.split(/\s+/); if (parts.length < 2) continue
          const name = parts[0].split('{')[0]; const val = parseFloat(parts[parts.length - 1]) || 0
          if (name.endsWith('scanned')) s = val; else if (name.endsWith('copied')) c = val; else if (name.endsWith('failed')) f = val; else if (name.endsWith('copied_bytes') || name.endsWith('transferred_bytes') || name.endsWith('copied_bytes_total')) b = val
        }
        setScanned(s); setCopied(c); setFailed(f); setBytesCopied(b)
        const delta = b - prevBytes.current; prevBytes.current = b
        if (delta > 0) setSpeedHistory(prev => { const next = [...prev, { t: new Date().toLocaleTimeString(), v: Math.round((delta / 1024 / 1024) * 10) / 10 }]; return next.slice(-20) })
        setStatus(s > 0 ? 'running' : 'idle')
      } catch { setStatus('unknown') }
    }; fetchMetrics(); prevBytes.current = 0; const interval = setInterval(fetchMetrics, 3000); return () => clearInterval(interval)
  }, [inst.port])
  const progress = scanned > 0 ? Math.min(100, Math.round((copied / scanned) * 100)) : 0
  const avgSpeed = speedHistory.length > 0 ? speedHistory.reduce((a, b) => a + b.v, 0) / speedHistory.length : 0
  return <div className="rounded-2xl border border-white/[0.06] bg-white/[0.02] backdrop-blur-xl p-6 hover:bg-white/[0.04] transition-all duration-300 hover:border-white/[0.12] hover:shadow-lg hover:shadow-indigo-500/5 tech-border" style={{ animationDelay: `${index * 80}ms` }}>
    <div className="flex items-start justify-between mb-5">
      <div className="min-w-0"><h3 className="font-semibold text-white truncate">{inst.name || '未命名'}</h3><p className="text-sm text-zinc-500 mt-1 truncate"><span className="text-cyan-400">{inst.src}</span><span className="mx-2 text-zinc-600">→</span><span className="text-purple-400">{inst.dst}</span></p></div>
      <StatusBadge status={status} />
    </div>
    <div className="mb-5">
      <div className="flex justify-between text-xs mb-2"><span className="text-zinc-500">同步进度</span><span className="text-indigo-400 font-mono font-medium">{progress}%</span></div>
      <div className="relative h-2 rounded-full bg-white/[0.06] overflow-hidden"><div className="h-full rounded-full bg-gradient-to-r from-indigo-500 via-purple-500 to-pink-500 transition-all duration-700 ease-out relative overflow-hidden" style={{ width: `${progress}%` }}><div className="absolute inset-0 bg-gradient-to-r from-transparent via-white/20 to-transparent animate-scan-line" /></div></div>
    </div>
    <div className="grid grid-cols-4 gap-4 mb-4">
      <MetricItem value={scanned} label="已扫描" icon={Activity} color="text-blue-400" />
      <MetricItem value={copied} label="已复制" icon={Copy} color="text-emerald-400" />
      <MetricItem value={failed} label="失败" icon={AlertCircle} color="text-red-400" />
      <MetricItem value={bytesCopied} label="传输量" icon={HardDrive} color="text-cyan-400" isBytes />
    </div>
    {speedHistory.length > 1 && <div className="mb-4">
      <div className="flex items-center justify-between text-xs mb-2"><span className="text-zinc-500 flex items-center gap-1"><Network className="w-3 h-3" /> 实时吞吐量</span><span className="text-cyan-400 font-mono font-medium">{avgSpeed.toFixed(1)} MB/s</span></div>
      <div className="h-12"><ResponsiveContainer width="100%" height="100%"><AreaChart data={speedHistory}><defs><linearGradient id="sg" x1="0" y1="0" x2="0" y2="1"><stop offset="0%" stopColor="#06f7f7" stopOpacity={0.3} /><stop offset="100%" stopColor="#06f7f7" stopOpacity={0} /></linearGradient></defs><Area type="monotone" dataKey="v" stroke="#06f7f7" strokeWidth={1.5} fill="url(#sg)" dot={false} /></AreaChart></ResponsiveContainer></div>
    </div>}
    <div className="flex justify-between items-center pt-3 border-t border-white/[0.06]">
      <code className="text-xs text-cyan-500/70 font-mono flex items-center gap-1"><Cpu className="w-3 h-3" /> :{inst.port}</code>
      <code className="text-xs text-zinc-600 font-mono flex items-center gap-1"><Timer className="w-3 h-3" /> PID {inst.pid}</code>
    </div>
  </div>
}
function StatusBadge({ status }: { status: string }) {
  const config: Record<string, { dot: string; label: string; ring: boolean }> = { running: { dot: 'bg-emerald-400 animate-pulse shadow-lg shadow-emerald-500/30', label: '运行中', ring: true }, idle: { dot: 'bg-amber-400', label: '空闲', ring: false }, unknown: { dot: 'bg-zinc-500', label: '未知', ring: false } }
  const c = config[status] || config.unknown
  return <span className="inline-flex items-center gap-2 px-3 py-1.5 rounded-full bg-white/[0.04] border border-white/[0.06] text-xs font-medium text-zinc-400 shrink-0">
    <span className={`w-2 h-2 rounded-full ${c.dot} ${c.ring ? 'animate-[pulse-ring_2s_ease-in-out_infinite]' : ''}`} />{c.label}</span>
}
function MetricItem({ value, label, icon: Icon, color, isBytes }: { value: number; label: string; icon: React.ComponentType<{ className?: string }>; color: string; isBytes?: boolean }) {
  const display = isBytes ? formatBytes(value) : value.toLocaleString()
  return <div className="text-center count-up-item"><Icon className={`w-4 h-4 mx-auto mb-1 ${color}`} /><div className={`text-lg font-bold tabular-nums ${color}`}>{display}</div><div className="text-[10px] text-zinc-600 uppercase tracking-widest mt-0.5 font-medium">{label}</div></div>
}
function formatBytes(bytes: number): string { if (!bytes) return '0 B'; const units = ['B', 'KB', 'MB', 'GB', 'TB']; let i = 0, size = bytes; while (size >= 1024 && i < units.length - 1) { size /= 1024; i++ }; return `${size.toFixed(1)} ${units[i]}` }
