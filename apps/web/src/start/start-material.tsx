import { useEffect, useState, type ComponentProps } from 'react'
import { usePreferences } from '../preferences'
import styles from './start-material.module.scss'

const loadBinding = () => import('./start-liquid')
type Binding = Awaited<ReturnType<typeof loadBinding>>['default']
type Surface = 'category' | 'search' | 'quick-icon' | 'calculator'

function LiquidSurface(props: ComponentProps<Binding>) {
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
}

export function useStartSurface(surface: Surface, representative = true) {
  const { startMaterial } = usePreferences()
  const enabled = representative && startMaterial === 'liquid'
  const [node, setNode] = useState<HTMLElement | null>(null)
  const [readyNode, setReadyNode] = useState<HTMLElement | null>(null)
  const ready = enabled && node !== null && readyNode === node

  return {
    enabled,
    ready,
    props: {
      ref: enabled ? setNode : undefined,
      'data-start-liquid': ready ? surface : undefined,
      'data-lg-material': ready ? 'clear' : undefined,
    },
    className: ready ? `liquid-glass ${styles.surface}` : undefined,
    binding: enabled && node && <LiquidSurface node={node} onReady={setReadyNode} />,
  }
}
