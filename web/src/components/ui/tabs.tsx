import * as React from "react"; import { cn } from "@/lib/utils"
const TabsContext = React.createContext<{ value: string; onValueChange: (v: string) => void } | null>(null)
function Tabs({ value, onValueChange, className, ...props }: React.HTMLAttributes<HTMLDivElement> & { value: string; onValueChange: (v: string) => void }) { return <TabsContext.Provider value={{ value, onValueChange }}><div className={cn("", className)} {...props} /></TabsContext.Provider> }
function TabsList({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) { return <div className={cn("inline-flex h-9 items-center justify-center rounded-lg bg-muted p-1 text-muted-foreground", className)} {...props} /> }
function TabsTrigger({ className, value, ...props }: React.HTMLAttributes<HTMLButtonElement> & { value: string }) {
  const ctx = React.useContext(TabsContext); const isActive = ctx?.value === value
  return <button className={cn("inline-flex items-center justify-center whitespace-nowrap rounded-md px-3 py-1 text-sm font-medium ring-offset-background transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50", isActive && "bg-background text-foreground shadow", className)} onClick={() => ctx?.onValueChange(value)} {...props} />
}
function TabsContent({ className, value, ...props }: React.HTMLAttributes<HTMLDivElement> & { value: string }) {
  const ctx = React.useContext(TabsContext); if (ctx?.value !== value) return null
  return <div className={cn("mt-2 ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2", className)} {...props} />
}
export { Tabs, TabsList, TabsTrigger, TabsContent }
