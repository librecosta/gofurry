import { memo, useEffect, useState, type ComponentProps } from 'react'
import { usePreferences } from '../preferences'
import styles from './start-material.module.scss'

const loadBinding = () => import('./start-liquid')
type Binding = Awaited<ReturnType<typeof loadBinding>>['default']
type Surface = 'category' | 'search' | 'quick-icon' | 'tool' | 'widget' | 'widget-manager'

const visibilityListeners = new Map<Element, (visible: boolean) => void>()
let visibilityObserver: IntersectionObserver | undefined

function observeSurface(node: HTMLElement, onVisibility: (visible: boolean) => void) {
  // Observe actual layout so CSS breakpoints, overflow and keyed reordering
  // all use the same visibility rule without duplicating the responsive CSS.
  const observer = visibilityObserver ??= new IntersectionObserver((entries, source) => {
    if (source !== visibilityObserver) return
    for (const entry of entries) {
      const { width, height } = entry.intersectionRect
      visibilityListeners.get(entry.target)?.(entry.isIntersecting && width > 0 && height > 0)
    }
    // Distinguish touching a viewport edge from showing some area.
  }, { threshold: [0, Number.EPSILON] })
  visibilityListeners.set(node, onVisibility)
  observer.observe(node)
  return () => {
    observer.unobserve(node)
    visibilityListeners.delete(node)
    if (visibilityListeners.size === 0) {
      observer.disconnect()
      visibilityObserver = undefined
    }
  }
}

const LiquidSurface = memo(function LiquidSurface(props: ComponentProps<Binding>) {
  const [Binding, setBinding] = useState<Binding>()
  useEffect(() => {
    let mounted = true
    // Frosted is already painted, so there is no Suspense fallback to reveal.
    void loadBinding().then(({ default: component }) => {
      if (mounted) setBinding(() => component)
    }, () => { /* Keep the original Frosted surface on a chunk failure. */ })
    return () => { mounted = false }
  }, [])
  return Binding ? <Binding {...props} /> : null
})

export function useStartSurface(surface: Surface, eligible = true) {
  const { startMaterial } = usePreferences()
  const enabled = eligible && startMaterial === 'liquid'
  const [node, setNode] = useState<HTMLElement | null>(null)
  const [visibleNode, setVisibleNode] = useState<HTMLElement | null>(null)
  const [readyNode, setReadyNode] = useState<HTMLElement | null>(null)
  const visible = enabled && node !== null && visibleNode === node
  const ready = visible && readyNode === node

  useEffect(() => {
    if (!enabled || !node || typeof IntersectionObserver === 'undefined') return
    const stop = observeSurface(node, inView => setVisibleNode(inView ? node : null))
    return () => {
      stop()
      setVisibleNode(null)
    }
  }, [enabled, node])

  return {
    enabled,
    ready,
    props: {
      ref: enabled ? setNode : undefined,
      'data-start-liquid': ready ? surface : undefined,
      'data-lg-material': ready ? 'clear' : undefined,
    },
    className: ready ? `liquid-glass ${styles.surface}` : undefined,
    binding: visible && <LiquidSurface node={node} onReady={setReadyNode} />,
  }
}
