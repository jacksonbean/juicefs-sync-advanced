import { useEffect, useRef, useState } from 'react'
export function useCountUp(end: number, duration = 800) {
  const [value, setValue] = useState(0); const prevEnd = useRef(0)
  useEffect(() => {
    const startVal = prevEnd.current; const startTime = performance.now()
    if (startVal === end) return
    const tick = (now: number) => {
      const elapsed = now - startTime; const progress = Math.min(elapsed / duration, 1)
      const eased = 1 - Math.pow(1 - progress, 3)
      setValue(Math.round(startVal + (end - startVal) * eased))
      if (progress < 1) requestAnimationFrame(tick)
    }; prevEnd.current = end; requestAnimationFrame(tick)
  }, [end, duration])
  return value
}
