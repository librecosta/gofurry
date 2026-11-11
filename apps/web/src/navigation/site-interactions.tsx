import { useEffect, type MouseEvent } from 'react'

type Interaction = 'detail_view' | 'outbound_click'

function report(publicID: string, type: Interaction) {
  const endpoint = `/api/v1/sites/${publicID}/interactions`
  const body = JSON.stringify({ type })
  try {
    if (navigator.sendBeacon?.(endpoint, new Blob([body], { type: 'application/json' }))) return
  } catch {
    // A failed Beacon never changes the navigation action.
  }
  void fetch(endpoint, {
    method: 'POST', body, headers: { 'Content-Type': 'application/json' },
    keepalive: true, credentials: 'omit', redirect: 'error',
  }).catch(() => undefined)
}

export function SiteDwell({ publicID }: { publicID: string }) {
  useEffect(() => {
    const required = 3000
    const page: Document & { prerendering?: boolean } = document
    let accrued = 0
    let started: number | undefined
    let timer: number | undefined
    let sent = false
    const tick = () => {
      window.clearTimeout(timer)
      const now = performance.now()
      if (started !== undefined) accrued += now - started
      started = undefined
      if (sent || page.visibilityState !== 'visible' || page.prerendering) return
      if (accrued >= required) {
        sent = true
        report(publicID, 'detail_view')
      } else {
        started = now
        timer = window.setTimeout(tick, required - accrued)
      }
    }
    document.addEventListener('visibilitychange', tick)
    document.addEventListener('prerenderingchange', tick)
    tick()
    return () => {
      window.clearTimeout(timer)
      document.removeEventListener('visibilitychange', tick)
      document.removeEventListener('prerenderingchange', tick)
    }
  }, [publicID])
  return null
}

export function siteOutbound(publicID: string, event: MouseEvent<HTMLAnchorElement>) {
  if (event.nativeEvent.isTrusted && !event.defaultPrevented &&
    ((event.type === 'click' && event.button === 0) || (event.type === 'auxclick' && event.button === 1))) {
    report(publicID, 'outbound_click')
  }
}
