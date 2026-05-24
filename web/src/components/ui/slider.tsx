import * as React from "react"; import { cn } from "@/lib/utils"
interface SliderProps extends Omit<React.HTMLAttributes<HTMLDivElement>, 'value' | 'defaultValue' | 'onValueChange'> { value?: number[]; defaultValue?: number[]; onValueChange?: (value: number[]) => void; min?: number; max?: number; step?: number }
const Slider = React.forwardRef<HTMLDivElement, SliderProps>(({ className, value, defaultValue, onValueChange, min = 0, max = 100, step = 1, ...props }, ref) => {
  const [internalValue, setInternalValue] = React.useState(defaultValue?.[0] ?? min)
  const currentValue = value?.[0] ?? internalValue; const percentage = ((currentValue - min) / (max - min)) * 100
  return <div ref={ref} className={cn("relative flex w-full touch-none select-none items-center", className)} {...props}>
    <div className="relative h-1.5 w-full rounded-full bg-primary/20"><div className="absolute h-full rounded-full bg-primary" style={{ width: `${percentage}%` }} /></div>
    <input type="range" min={min} max={max} step={step} value={currentValue} onChange={e => { const v = parseInt(e.target.value); setInternalValue(v); onValueChange?.([v]) }} className="absolute inset-0 w-full opacity-0 cursor-pointer" />
  </div>
}); Slider.displayName = "Slider"; export { Slider }
