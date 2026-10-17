import { ref } from 'vue'
import type { PackageJob } from '../types'
import { wsService } from './websocket'

const POLL_MS = 1000
/** Poll a running job when the socket has said nothing about it for this long. */
const STALE_MS = 2500

export function isActive(job: PackageJob | null | undefined): boolean {
  return !!job && (job.state === 'queued' || job.state === 'running')
}

/** The more recent of two jobs (by start time), preferring a defined one. */
export function newerJob(a: PackageJob | null | undefined, b: PackageJob | null | undefined): PackageJob | null {
  if (!a) return b || null
  if (!b) return a
  if (a.id === b.id) return a.log.length >= b.log.length ? a : b
  return (a.started || '') >= (b.started || '') ? a : b
}

/**
 * Keeps background jobs live: WebSocket events of `eventType` update them as they stream, and a
 * 1s poll takes over for any running job the socket has gone quiet about (disconnected, or the
 * daemon does not emit for it). `onFinished` runs once when a job leaves queued/running.
 *
 * Views call attach() on mount and the returned detach on unmount; the socket listener and the
 * poll timer only exist while at least one view is attached.
 */
export function createJobTracker(
  eventType: string,
  fetchJob: (id: string) => Promise<PackageJob>,
  onFinished: (job: PackageJob) => void,
) {
  const jobs = ref<Record<string, PackageJob>>({})
  const seen = new Map<string, number>()
  const finished = new Set<string>()
  let attached = 0
  let unsubscribe: (() => void) | null = null
  let timer: ReturnType<typeof setInterval> | null = null
  let polling = false

  function put(job: PackageJob) {
    if (!job?.id) return
    const prev = jobs.value[job.id]
    jobs.value = { ...jobs.value, [job.id]: job }
    seen.set(job.id, Date.now())
    if (!isActive(job) && !finished.has(job.id)) {
      finished.add(job.id)
      // Only announce jobs we watched run, not old finished ones a list happened to carry.
      if (prev && isActive(prev)) onFinished(job)
    }
    ensureTimer()
  }

  /**
   * Starts following a job: one an action just started (`started`: announce it even when it
   * already ended), or one a list says exists (announce only if we then watch it end).
   */
  function track(job: PackageJob | null | undefined, started = false) {
    if (!job?.id) return
    const prev = jobs.value[job.id]
    // A list snapshot must not overwrite a fresher streamed copy.
    if (prev && prev.log.length > (job.log?.length || 0) && isActive(job)) return
    if (!prev && !isActive(job)) {
      jobs.value = { ...jobs.value, [job.id]: job }
      if (!finished.has(job.id)) {
        finished.add(job.id)
        if (started) onFinished(job)
      }
      return
    }
    put(job)
  }

  function anyActive(): boolean {
    return Object.values(jobs.value).some(isActive)
  }

  function ensureTimer() {
    if (attached > 0 && anyActive() && !timer) {
      timer = setInterval(() => void poll(), POLL_MS)
    } else if ((attached === 0 || !anyActive()) && timer) {
      clearInterval(timer)
      timer = null
    }
  }

  async function poll() {
    if (polling) return
    polling = true
    try {
      const now = Date.now()
      const due = Object.values(jobs.value).filter(
        (j) => isActive(j) && (!wsService.isConnected.value || now - (seen.get(j.id) || 0) > STALE_MS),
      )
      for (const j of due) {
        try {
          put(await fetchJob(j.id))
        } catch {
          // the daemon restarted or the network blinked: try again next tick
        }
      }
    } finally {
      polling = false
      ensureTimer()
    }
  }

  function attach(): () => void {
    attached++
    if (!unsubscribe) {
      unsubscribe = wsService.subscribe((event) => {
        if (event.type !== eventType || !event.payload?.id) return
        put(event.payload as PackageJob)
      })
    }
    ensureTimer()
    let done = false
    return () => {
      if (done) return
      done = true
      attached--
      if (attached === 0 && unsubscribe) {
        unsubscribe()
        unsubscribe = null
      }
      ensureTimer()
    }
  }

  return { jobs, track, attach }
}
