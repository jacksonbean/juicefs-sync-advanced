import * as React from "react"; import { cn } from "@/lib/utils"
interface SelectContextType { value: string; onValueChange: (v: string) => void; open: boolean; setOpen: (v: boolean) => void }
const SelectContext = React.createContext<SelectContextType | null>(null)
function Select({ value, onValueChange, children }: { value?: string; onValueChange?: (v: string) => void; children: React.ReactNode }) {
  const [open, setOpen] = React.useState(false); const [internalValue, setInternalValue] = React.useState("")
  return <SelectContext.Provider value={{ value: value ?? internalValue, onValueChange: onValueChange ?? setInternalValue, open, setOpen }}><div className="relative">{children}</div></SelectContext.Provider>
}
const SelectTrigger = React.forwardRef<HTMLButtonElement, React.ButtonHTMLAttributes<HTMLButtonElement>>(({ className, children, ...props }, ref) => {
  const ctx = React.useContext(SelectContext)
  return <button ref={ref} className={cn("flex h-9 w-full items-center justify-between rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-sm ring-offset-background placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring disabled:cursor-not-allowed disabled:opacity-50", className)} onClick={() => ctx?.setOpen(!ctx?.open)} {...props}>{children}<span className="ml-2 opacity-50">▼</span></button>
}); SelectTrigger.displayName = "SelectTrigger"
const SelectValue = React.forwardRef<HTMLSpanElement, React.HTMLAttributes<HTMLSpanElement>>(({ className, children, ...props }, ref) => {
  const ctx = React.useContext(SelectContext); const displayValue = ctx?.value || (typeof children === 'string' ? children : '')
  return <span ref={ref} className={cn("block truncate", className)} {...props}>{displayValue || (children as React.ReactNode)}</span>
}); SelectValue.displayName = "SelectValue"
function SelectContent({ children, className }: { children: React.ReactNode; className?: string }) {
  const ctx = React.useContext(SelectContext)
  if (!ctx?.open) return null
  return <div className="absolute z-50 mt-1 w-full rounded-md border bg-popover p-1 shadow-md">
    <div className={cn("", className)} onClick={() => ctx?.setOpen(false)}>{children}</div>
  </div>
}
function SelectItem({ value, children, className }: { value: string; children: React.ReactNode; className?: string }) {
  const ctx = React.useContext(SelectContext); const isSelected = ctx?.value === value
  return <div className={cn("relative flex cursor-default select-none items-center rounded-sm px-2 py-1.5 text-sm outline-none hover:bg-accent hover:text-accent-foreground", isSelected && "bg-accent text-accent-foreground", className)} onClick={() => { ctx?.onValueChange(value); ctx?.setOpen(false) }}>{children}</div>
}
export { Select, SelectTrigger, SelectValue, SelectContent, SelectItem }
