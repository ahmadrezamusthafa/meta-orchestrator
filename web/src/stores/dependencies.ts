import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { PackageJob, ProjectDependencies, RepoDependencies } from '../types'
import { api, type DependencyInstallResult } from '../services/api'
import { createJobTracker, newerJob } from '../services/jobTracker'
import { useToastStore } from './toast'

export interface DependencyScope {
  projectId: string
  task?: string
}

function keyOf(scope: DependencyScope): string {
  return `${scope.projectId}|${scope.task || ''}`
}

/** Each project's repository dependencies (GET /projects/{id}/dependencies) and their install jobs. */
export const useDependenciesStore = defineStore('dependencies', () => {
  const toast = useToastStore()
  const reports = ref<Record<string, ProjectDependencies>>({})
  const loading = ref<Record<string, boolean>>({})
  const errors = ref<Record<string, string>>({})
  /** Reports loaded with ?latest=1 keep asking for it on refresh. */
  const withLatest = new Set<string>()
  const scopes = new Map<string, DependencyScope>()

  const tracker = createJobTracker('dependency.job', api.getDependencyJob, (job) => {
    const repo = job.package.replace(/^repo:/, '')
    if (job.state === 'succeeded') toast.success(`Dependencies installed in ${repo}`, job.message || undefined)
    else toast.error(`Dependency install failed in ${repo}`, job.message || undefined)
    for (const [key, report] of Object.entries(reports.value)) {
      if (report.repos.some((r) => r.repo === repo)) {
        const scope = scopes.get(key)
        if (scope) void load(scope)
      }
    }
  })

  function report(scope: DependencyScope): ProjectDependencies | null {
    return reports.value[keyOf(scope)] || null
  }

  function jobFor(repo: RepoDependencies): PackageJob | null {
    let best: PackageJob | null = repo.job ? tracker.jobs.value[repo.job.id] || repo.job : null
    const pkg = `repo:${repo.repo}`
    for (const j of Object.values(tracker.jobs.value)) if (j.package === pkg) best = newerJob(best, j)
    return best
  }

  async function load(scope: DependencyScope, latest?: boolean) {
    const key = keyOf(scope)
    scopes.set(key, scope)
    if (latest) withLatest.add(key)
    loading.value = { ...loading.value, [key]: true }
    errors.value = { ...errors.value, [key]: '' }
    try {
      const res = await api.getProjectDependencies(scope.projectId, { task: scope.task, latest: withLatest.has(key) })
      reports.value = { ...reports.value, [key]: res }
      for (const r of res.repos || []) if (r.job) tracker.track(r.job)
    } catch (e: any) {
      errors.value = { ...errors.value, [key]: e?.message || 'Could not load dependencies' }
    } finally {
      loading.value = { ...loading.value, [key]: false }
    }
  }

  async function install(scope: DependencyScope, repo: string, ecosystem: string): Promise<DependencyInstallResult | null> {
    try {
      const res = await api.installProjectDependencies(scope.projectId, { repo, ecosystem, task: scope.task })
      if (res.kind === 'job') tracker.track(res.job, true)
      return res
    } catch (e: any) {
      toast.error(`Could not install dependencies in ${repo}`, e?.message)
      return null
    }
  }

  return {
    reports,
    loading,
    errors,
    jobs: tracker.jobs,
    attach: tracker.attach,
    keyOf,
    report,
    jobFor,
    load,
    install,
  }
})
