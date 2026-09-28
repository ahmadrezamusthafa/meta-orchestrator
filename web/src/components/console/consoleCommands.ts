export interface SlashCommand {
  name: string
  args?: string
  description: string
}

export const SLASH_COMMANDS: SlashCommand[] = [
  { name: 'help', description: 'Show available commands and shortcuts' },
  { name: 'clear', description: 'Clear the activity transcript for this task' },
  { name: 'run', description: 'Execute the current stage (routes through the priority chain)' },
  { name: 'resume', description: 'Resume a paused or blocked session' },
  { name: 'pause', description: 'Pause the running session' },
  { name: 'approve', args: '[feedback]', description: 'Approve the pending gate and advance' },
  { name: 'reject', args: '[feedback]', description: 'Reject the pending gate with feedback' },
  { name: 'status', description: 'Show task state, stage, model and token usage' },
  { name: 'cost', description: 'Show session token usage and cost' },
  { name: 'model', args: '<provider/model>', description: 'Set the model for subsequent messages (no arg: show, "default": reset)' },
  { name: 'export', description: 'Download the transcript as markdown' },
  { name: 'expand', description: 'Expand all tool results and requests' },
  { name: 'collapse', description: 'Collapse all tool results and requests' },
]

export const SHORTCUTS: Array<{ keys: string; description: string }> = [
  { keys: 'enter', description: 'send message / run command' },
  { keys: 'shift+enter', description: 'insert newline' },
  { keys: '↑ / ↓', description: 'recall input history (at first/last line)' },
  { keys: 'esc', description: 'interrupt running turn · clear input' },
  { keys: '/', description: 'open command menu (tab / enter to pick)' },
  { keys: '?', description: 'toggle this panel (on empty input)' },
]

export function filterCommands(input: string): SlashCommand[] {
  if (!input.startsWith('/')) return []
  const head = input.slice(1)
  if (/\s/.test(head)) return []
  const q = head.toLowerCase()
  return SLASH_COMMANDS.filter(c => c.name.startsWith(q))
}

export function helpText(): string {
  const width = Math.max(...SLASH_COMMANDS.map(c => (c.name + (c.args ? ' ' + c.args : '')).length)) + 3
  const lines = ['Commands', '']
  for (const c of SLASH_COMMANDS) {
    const sig = `/${c.name}${c.args ? ' ' + c.args : ''}`
    lines.push(`  ${sig.padEnd(width)}${c.description}`)
  }
  lines.push('', 'Shortcuts', '')
  for (const s of SHORTCUTS) lines.push(`  ${s.keys.padEnd(width)}${s.description}`)
  lines.push('', 'Anything not starting with / is sent to the agent as a chat message.')
  return lines.join('\n')
}
