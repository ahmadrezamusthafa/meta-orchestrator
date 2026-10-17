import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { ToolchainItem, ToolchainJob, ToolchainReport } from '../types'
import { toolchainApi } from '../services/toolchain'
import { useToastStore } from './toast'

/** A stage that waits for tools, and how to start it again once they are installed. */
export interface ToolchainPrompt {
  taskId: string
  stageId?: string
  retry: 'run' | 'resume'
  report: ToolchainReport | null
}

const POLL_MS = 1000

export const useToolchainStore = defineStore('toolchain', () => {
  const toast = useToastStore()
  /** Latest install job per tool id, kept live while it runs. */
  const jobs = ref<Record<string, ToolchainJob>>({})
  /** Bumped whenever an install finishes, so every toolchain view re-checks. */
  const revision = ref(0)
  const prompt = ref<ToolchainPrompt | null>(null)
  const polling = new Set<string>()

  function jobFor(item: ToolchainItem): ToolchainJob | null {
    const local = jobs.value[item.id]
    const remote = item.job || null
    if (!local) return remote
    if (!remote) return local
    return local.started >= remote.started ? local : remote
  }

  function isInstalling(toolId: string): boolean {
    return jobs.value[toolId]?.state === 'running'
  }

  /** Follows a job until it finishes, then tells the operator how it went. */
  async function follow(job: ToolchainJob, name: string): Promise<ToolchainJob> {
    jobs.value = { ...jobs.value, [job.tool]: job }
    if (job.state !== 'running' || polling.has(job.id)) return job
    polling.add(job.id)
    try {
      let current = job
      while (current.state === 'running') {
        await new Promise((r) => setTimeout(r, POLL_MS))
        try {
          current = await toolchainApi.job(job.id)
        } catch {
          continue // the daemon restarted or the network blinked: keep trying
        }
        jobs.value = { ...jobs.value, [job.tool]: current }
      }
      if (current.state === 'succeeded') toast.success(`${name} installed`, current.message)
      else toast.error(`${name} was not installed`, current.message)
      revision.value++
      return current
    } finally {
      polling.delete(job.id)
    }
  }

  /** Installs a tool with one of its one-click methods and resolves when the install ends. */
  async function install(item: ToolchainItem, method: number): Promise<ToolchainJob | null> {
    try {
      const job = await toolchainApi.install(item.id, method)
      return await follow(job, item.name)
    } catch (err: any) {
      toast.error(`Could not install ${item.name}`, err?.message)
      return null
    }
  }

  /** Resumes following installs that were already running when a view loaded. */
  function resume(items: ToolchainItem[]) {
    for (const it of items) {
      if (it.job?.state === 'running') void follow(it.job, it.name)
    }
  }

  async function openPrompt(taskId: string, opts: { stageId?: string; retry?: 'run' | 'resume'; report?: ToolchainReport | null } = {}) {
    if (prompt.value?.taskId === taskId && prompt.value.report) return // already asking about this task
    prompt.value = { taskId, stageId: opts.stageId, retry: opts.retry || 'run', report: opts.report || null }
    if (!opts.report) {
      try {
        const report = await toolchainApi.taskReport(taskId)
        if (prompt.value?.taskId === taskId) prompt.value = { ...prompt.value, report }
      } catch (err: any) {
        toast.error('Could not check the tools', err?.message)
        prompt.value = null
      }
    }
  }

  function closePrompt() {
    prompt.value = null
  }

  return { jobs, revision, prompt, jobFor, isInstalling, install, resume, openPrompt, closePrompt }
})
