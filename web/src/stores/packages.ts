import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import type { PackageDTO, PackageJob, PackageManual, PackagesResponse } from '../types'
import { api } from '../services/api'
import { createJobTracker, isActive, newerJob } from '../services/jobTracker'
import { useToastStore } from './toast'

const VERB: Record<string, [string, string]> = {
  install: ['installed', 'install failed'],
  update: ['updated', 'update failed'],
  rollback: ['rolled back', 'rollback failed'],
  uninstall: ['uninstalled', 'uninstall failed'],
  setup: ['set up', 'setup failed'],
}

/** The tools the orchestrator itself uses (GET /packages), and their install/update/rollback jobs. */
export const usePackagesStore = defineStore('packages', () => {
  const toast = useToastStore()
  const data = ref<PackagesResponse | null>(null)
  const loading = ref(false)
  const error = ref('')
  /** Jobs started by the last "Set up everything", in chain order. */
  const setupJobIds = ref<string[]>([])
  const setupSkipped = ref<{ id: string; reason: string }[]>([])
  const settingUp = ref(false)

  let refreshTimer: ReturnType<typeof setTimeout> | null = null

  const tracker = createJobTracker('package.job', api.getPackageJob, (job) => {
    const name = nameOf(job.package)
    const [ok, bad] = VERB[job.action] || ['done', 'failed']
    if (job.state === 'succeeded') toast.success(`${name} ${ok}`, job.message || undefined)
    else toast.error(`${name} ${bad}`, job.message || undefined)
    scheduleRefresh()
  })

  const packages = computed(() => data.value?.packages || [])
  const missingRequired = computed(() => packages.value.filter((p) => p.required && p.status !== 'installed'))
  const setupJobs = computed(() =>
    setupJobIds.value.map((id) => tracker.jobs.value[id]).filter((j): j is PackageJob => !!j),
  )
  const setupRunning = computed(() => setupJobs.value.some(isActive))

  function nameOf(id: string): string {
    return packages.value.find((p) => p.id === id)?.name || id
  }

  /** The job to show for a package: the newest one we follow, or the one the list reported. */
  function jobFor(pkg: PackageDTO): PackageJob | null {
    let best: PackageJob | null = pkg.job ? tracker.jobs.value[pkg.job.id] || pkg.job : null
    for (const j of Object.values(tracker.jobs.value)) {
      if (j.package === pkg.id) best = newerJob(best, j)
    }
    return best
  }

  function absorb(res: PackagesResponse) {
    data.value = res
    for (const p of res.packages || []) if (p.job) tracker.track(p.job)
  }

  async function fetch() {
    loading.value = true
    error.value = ''
    try {
      absorb(await api.getPackages())
    } catch (e: any) {
      error.value = e?.message || 'Could not load packages'
    } finally {
      loading.value = false
    }
  }

  async function recheck() {
    loading.value = true
    error.value = ''
    try {
      absorb(await api.recheckPackages())
    } catch (e: any) {
      error.value = e?.message || 'Could not re-check packages'
      throw e
    } finally {
      loading.value = false
    }
  }

  /** Several jobs often end together (a setup chain): refresh the list once for them. */
  function scheduleRefresh() {
    if (refreshTimer) clearTimeout(refreshTimer)
    refreshTimer = setTimeout(() => {
      refreshTimer = null
      void fetch()
    }, 300)
  }

  async function run(pkg: PackageDTO, what: string, start: () => Promise<PackageJob>): Promise<PackageJob | null> {
    try {
      const job = await start()
      tracker.track(job, true)
      return job
    } catch (e: any) {
      toast.error(`Could not ${what} ${pkg.name}`, e?.message)
      return null
    }
  }

  /** Installs (optionally a specific version). Returns manual steps when it cannot be automated. */
  async function install(pkg: PackageDTO, version?: string): Promise<{ manual: PackageManual | null; error: string } | null> {
    try {
      const res = await api.installPackage(pkg.id, version || undefined)
      if (res.kind === 'manual') return { manual: res.manual, error: res.error }
      tracker.track(res.job, true)
    } catch (e: any) {
      toast.error(`Could not install ${pkg.name}`, e?.message)
    }
    return null
  }

  const update = (pkg: PackageDTO) => run(pkg, 'update', () => api.updatePackage(pkg.id))
  const rollback = (pkg: PackageDTO, version?: string) => run(pkg, 'roll back', () => api.rollbackPackage(pkg.id, version))
  const uninstall = (pkg: PackageDTO) => run(pkg, 'uninstall', () => api.uninstallPackage(pkg.id))

  /** Installs every missing required package (POST /packages/setup), or every missing package of a
   *  category, and follows each job. */
  async function setup(category?: string) {
    settingUp.value = true
    try {
      const res = await api.setupPackages(category)
      setupSkipped.value = res.skipped || []
      setupJobIds.value = (res.jobs || []).map((j) => j.id)
      for (const j of res.jobs || []) tracker.track(j, true)
      if (!res.jobs?.length) {
        if (setupSkipped.value.length) toast.warning('Nothing could be set up automatically', 'See the skipped packages for why.')
        else toast.info(category ? `Every ${category} package is already installed` : 'Everything required is already installed')
        scheduleRefresh()
      }
    } catch (e: any) {
      toast.error('Could not start the setup', e?.message)
    } finally {
      settingUp.value = false
    }
  }

  return {
    data,
    packages,
    loading,
    error,
    missingRequired,
    setupJobs,
    setupSkipped,
    setupRunning,
    settingUp,
    jobs: tracker.jobs,
    attach: tracker.attach,
    jobFor,
    nameOf,
    fetch,
    recheck,
    install,
    update,
    rollback,
    uninstall,
    setup,
  }
})
