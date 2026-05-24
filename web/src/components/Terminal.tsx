interface TerminalProps { children: string; className?: string }
export default function Terminal({ children, className = '' }: TerminalProps) {
  return (
    <div className={`terminal ${className}`}>
      <div className="terminal-header">
        <span className="terminal-dot red" /><span className="terminal-dot yellow" /><span className="terminal-dot green" />
        <span className="ml-2 text-[10px] text-zinc-500 font-mono tracking-wider">TERMINAL</span>
      </div>
      <div className="terminal-body"><span className="text-cyan-500 select-none">$ </span>{children}<span className="typing-cursor inline-block" /></div>
    </div>
  )
}
