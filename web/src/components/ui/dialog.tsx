import * as React from "react"; import { cn } from "@/lib/utils"
interface DialogContextType { open: boolean; onOpenChange: (open: boolean) => void }
const DialogContext = React.createContext<DialogContextType>({ open: false, onOpenChange: () => {} })
function Dialog({ open: controlledOpen, onOpenChange, children }: { open?: boolean; onOpenChange?: (open: boolean) => void; children: React.ReactNode }) {
  const [internalOpen, setInternalOpen] = React.useState(false)
  const open = controlledOpen ?? internalOpen; const setOpen = onOpenChange ?? setInternalOpen
  return <DialogContext.Provider value={{ open, onOpenChange: setOpen }}><div>{children}</div></DialogContext.Provider>
}
function DialogTrigger({ children, asChild }: { children: React.ReactNode; asChild?: boolean }) {
  const { onOpenChange } = React.useContext(DialogContext); return <div onClick={() => onOpenChange(true)}>{children}</div>
}
function DialogContent({ className, children, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  const { open, onOpenChange } = React.useContext(DialogContext)
  if (!open) return null
  return <div className="fixed inset-0 z-50 flex items-center justify-center">
    <div className="fixed inset-0 bg-black/50" onClick={() => onOpenChange(false)} />
    <div className={cn("relative z-50 w-full max-w-lg rounded-xl border bg-background p-6 shadow-lg", className)} {...props}>
      <button className="absolute right-4 top-4 rounded-sm opacity-70 ring-offset-background transition-opacity hover:opacity-100" onClick={() => onOpenChange(false)}>✕</button>
      {children}
    </div>
  </div>
}
function DialogHeader({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) { return <div className={cn("flex flex-col space-y-1.5 text-center sm:text-left", className)} {...props} /> }
function DialogTitle({ className, ...props }: React.HTMLAttributes<HTMLHeadingElement>) { return <h2 className={cn("text-lg font-semibold leading-none tracking-tight", className)} {...props} /> }
export { Dialog, DialogTrigger, DialogContent, DialogHeader, DialogTitle }
