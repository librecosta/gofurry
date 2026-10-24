import { useEffect } from 'react'

// Module-entry SSR apps need both the initial stylesheet and the HMR subscriber.
export function DevStyleX() {
  useEffect(() => {
    if (import.meta.env.DEV) void import('virtual:stylex:css-only')
  }, [])
  return import.meta.env.DEV ? <link rel="stylesheet" href="/virtual:stylex.css" /> : null
}
