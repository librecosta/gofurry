import { createContext, useContext, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { usePreferences } from '../preferences'
import { registeredWidgets, type WidgetId } from './widget-catalog'
import { readWidgetOrder, sameWidgetSet, toggleWidgetInOrder } from './widget-order'

type Rail = 'leftEnabled' | 'rightEnabled'
type RailState = { version: 1; leftEnabled: boolean; rightEnabled: boolean; widgets: WidgetId[] }
const defaults: RailState = { version: 1, leftEnabled: true, rightEnabled: true, widgets: [] }

function readState(raw: string | null): RailState {
  if (!raw || raw.length > 8192) return defaults
  try {
    const parsed = JSON.parse(raw)
    if (parsed?.version !== 1) return defaults
    return {
      version: 1,
      leftEnabled: parsed.leftEnabled !== false,
      rightEnabled: parsed.rightEnabled !== false,
      widgets: readWidgetOrder(parsed.widgets, registeredWidgets),
    }
  } catch { return defaults }
}

type WidgetRailsContext = RailState & {
  setEnabled: (rail: Rail, enabled: boolean) => void
  toggleWidget: (id: WidgetId) => void
  reorder: (ids: WidgetId[]) => void
}
const WidgetRailsContext = createContext<WidgetRailsContext | null>(null)

export function WidgetRailsProvider({ children }: { children: ReactNode }) {
  const { region } = usePreferences()
  const key = `gf-widget-rails:${region}`
  const [state, setState] = useState(defaults)
  const latest = useRef(state)

  useLayoutEffect(() => {
    let storage: Storage
    try { storage = window.localStorage } catch { return }
    const restore = (raw: string | null) => {
      latest.current = readState(raw)
      setState(latest.current)
    }
    try { restore(storage.getItem(key)) } catch { /* Keep the current state in memory. */ }
    const sync = (event: StorageEvent) => {
      if (event.storageArea === storage && (event.key === key || event.key === null)) restore(event.newValue)
    }
    window.addEventListener('storage', sync)
    return () => window.removeEventListener('storage', sync)
  }, [key])

  const commit = (next: RailState) => {
    latest.current = next
    setState(next)
    try { localStorage.setItem(key, JSON.stringify(next)) } catch { /* Memory-only fallback. */ }
  }

  return <WidgetRailsContext value={{
    ...state,
    setEnabled: (rail, enabled) => commit({ ...latest.current, [rail]: enabled }),
    toggleWidget: id => {
      const { widgets } = latest.current
      const next = toggleWidgetInOrder(widgets, id, registeredWidgets)
      if (next !== widgets) commit({ ...latest.current, widgets: next })
    },
    reorder: widgets => {
      if (widgets !== latest.current.widgets && sameWidgetSet(latest.current.widgets, widgets)) commit({ ...latest.current, widgets })
    },
  }}>{children}</WidgetRailsContext>
}

export function useWidgetRails() {
  const context = useContext(WidgetRailsContext)
  if (!context) throw new Error('WidgetRailsProvider is required')
  return context
}
