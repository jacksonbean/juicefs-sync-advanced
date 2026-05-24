import * as React from "react"; import { cn } from "@/lib/utils"
const Sheet = ({ children, open, onOpenChange }: { children: React.ReactNode; open?: boolean; onOpenChange?: (v: boolean) => void }) => { return <div>{children}</div> }
const SheetTrigger = ({ children, asChild, onClick }: { children: React.ReactNode; asChild?: boolean; onClick?: () => void }) => <div onClick={onClick}>{children}</div>
const SheetContent = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement> & { side?: 'left' | 'right' | 'top' | 'bottom' }>(({ className, children, side = 'right', ...props }, ref) => <div ref={ref} className={cn("fixed inset-y-0 right-0 z-50 w-3/4 max-w-sm border-l bg-background p-6 shadow-lg", className)} {...props}>{children}</div>); SheetContent.displayName = "SheetContent"
const SheetHeader = ({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) => <div className={cn("flex flex-col space-y-2 text-center sm:text-left", className)} {...props} />
const SheetTitle = React.forwardRef<HTMLHeadingElement, React.HTMLAttributes<HTMLHeadingElement>>(({ className, ...props }, ref) => <h2 ref={ref} className={cn("text-lg font-semibold text-foreground", className)} {...props} />); SheetTitle.displayName = "SheetTitle"
export { Sheet, SheetTrigger, SheetContent, SheetHeader, SheetTitle }
