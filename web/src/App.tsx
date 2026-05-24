import { useEffect, useState } from 'react'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { LayoutDashboard, Rocket, FileText, Clock, History, AlertTriangle, RotateCcw } from 'lucide-react'
import Dashboard from './pages/Dashboard'
import NewTask from './pages/NewTask'
import Templates from './pages/Templates'
import Schedule from './pages/Schedule'
import HistoryPage from './pages/History'
import Failed from './pages/Failed'
const TABS = [
  { id: 'dashboard', label: '仪表盘', icon: LayoutDashboard, component: Dashboard },
  { id: 'new-task', label: '新任务', icon: Rocket, component: NewTask },
  { id: 'templates', label: '模板', icon: FileText, component: Templates },
  { id: 'schedule', label: '调度', icon: Clock, component: Schedule },
  { id: 'history', label: '历史', icon: History, component: HistoryPage },
  { id: 'failed', label: '失败', icon: AlertTriangle, component: Failed },
]
export default function App() {
  const [tab, setTab] = useState(() => localStorage.getItem('active_tab') || 'dashboard')
  useEffect(() => { localStorage.setItem('active_tab', tab) }, [tab])
  return (
    <div className="min-h-screen bg-gradient-to-b from-zinc-950 via-zinc-950 to-zinc-900 grid-bg relative">
      <div className="ambient-orb-1" /><div className="ambient-orb-2" />
      <div className="relative z-10">
        <div className="h-[2px] bg-gradient-to-r from-indigo-500 via-purple-500 to-pink-500 relative overflow-hidden">
          <div className="absolute inset-0 w-1/3 bg-gradient-to-r from-transparent via-white/30 to-transparent animate-scan-line" />
        </div>
        <header className="border-b border-white/5">
          <div className="max-w-7xl mx-auto px-6 py-5">
            <div className="flex items-center gap-4">
              <div className="w-10 h-10 rounded-xl bg-gradient-to-br from-indigo-500 to-purple-600 flex items-center justify-center shadow-lg shadow-indigo-500/20">
                <RotateCcw className="w-5 h-5 text-white" />
              </div>
              <div>
                <h1 className="text-xl font-bold bg-gradient-to-r from-white to-purple-300 bg-clip-text text-transparent">JuiceFS Sync Advanced</h1>
                <p className="text-xs text-zinc-500 mt-0.5">Enterprise-grade object storage synchronization</p>
              </div>
            </div>
          </div>
        </header>
        <main className="max-w-7xl mx-auto px-6 py-6">
          <Tabs value={tab} onValueChange={setTab} className="space-y-6">
            <TabsList className="w-full inline-flex h-auto gap-1 bg-white/[0.03] border border-white/[0.06] p-1 rounded-xl backdrop-blur-xl">
              {TABS.map(t => (
                <TabsTrigger key={t.id} value={t.id} className="flex-1 min-w-[80px] py-2.5 text-sm data-[state=active]:bg-gradient-to-r data-[state=active]:from-indigo-500/20 data-[state=active]:to-purple-500/20 data-[state=active]:text-white data-[state=active]:shadow-sm rounded-lg transition-all">
                  <t.icon className="w-4 h-4 mr-1.5 inline-block" />{t.label}
                </TabsTrigger>
              ))}
            </TabsList>
            {TABS.map(t => { const C = t.component; return <TabsContent key={t.id} value={t.id} className="mt-0 animate-[fade-in_0.3s_ease-out]"><C /></TabsContent> })}
          </Tabs>
        </main>
      </div>
    </div>
  )
}
