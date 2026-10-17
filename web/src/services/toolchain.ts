import type { ToolchainJob, ToolchainReport } from '../types'

const BASE_URL = '/api/v1'

async function json<T>(res: Response, what: string): Promise<T> {
  if (!res.ok) {
    const err = await res.json().catch(() => null)
    throw new Error(err?.error || `${what} failed (HTTP ${res.status})`)
  }
  return res.json()
}

/** Scope of a report: one task, one project, or every registered repository. */
export interface ToolchainScope {
  task?: string
  project?: string
  /** Include every tool the orchestrator knows, not only the needed ones. */
  all?: boolean
}

function query(scope: ToolchainScope = {}): string {
  const q = new URLSearchParams()
  if (scope.task) q.set('task', scope.task)
  if (scope.project) q.set('project', scope.project)
  if (scope.all) q.set('all', '1')
  const s = q.toString()
  return s ? `?${s}` : ''
}

export const toolchainApi = {
  async report(scope?: ToolchainScope): Promise<ToolchainReport> {
    return json(await fetch(`${BASE_URL}/toolchain${query(scope)}`), 'Loading the toolchain')
  },
  /** Forgets cached checks (e.g. after installing from your own terminal) and reports again. */
  async recheck(scope?: ToolchainScope): Promise<ToolchainReport> {
    return json(await fetch(`${BASE_URL}/toolchain/recheck${query(scope)}`, { method: 'POST' }), 'Re-checking tools')
  },
  async install(tool: string, method: number): Promise<ToolchainJob> {
    const res = await fetch(`${BASE_URL}/toolchain/install`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ tool, method }),
    })
    if (res.status === 409) {
      const body = await res.json().catch(() => null)
      if (body?.job) return body.job // already installing: follow that job
    }
    return json(res, 'Starting the install')
  },
  async job(id: string): Promise<ToolchainJob> {
    return json(await fetch(`${BASE_URL}/toolchain/jobs/${encodeURIComponent(id)}`), 'Reading the install')
  },
  async taskReport(taskId: string): Promise<ToolchainReport> {
    return json(await fetch(`${BASE_URL}/tasks/${encodeURIComponent(taskId)}/toolchain`), 'Loading the task toolchain')
  },
  /** Lets the task's code stages run without the tools that are missing right now. */
  async acknowledge(taskId: string): Promise<ToolchainReport> {
    return json(await fetch(`${BASE_URL}/tasks/${encodeURIComponent(taskId)}/toolchain/ack`, { method: 'POST' }), 'Saving your choice')
  },
}
