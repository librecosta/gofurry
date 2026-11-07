import { Component, Suspense, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { CircleNotchIcon } from '@phosphor-icons/react/dist/ssr/CircleNotch'
import { motion } from 'motion/react'
import { useMotionTiming } from '../motion'
import { cx } from '../styles/classes'
import controls from './controls.module.scss'
import { useRailMessages } from './rail-messages'
import styles from './side-rails.module.scss'

export function RailContent({ children }: { children: ReactNode }) {
  const [loadingHeight, setLoadingHeight] = useState<number>()
  return <LoadBoundary>
    <Suspense fallback={<Loading measure={setLoadingHeight} />}>
      <Loaded from={loadingHeight}>{children}</Loaded>
    </Suspense>
  </LoadBoundary>
}

function Loading({ measure }: { measure: (height: number) => void }) {
  const { loading } = useRailMessages()
  const status = useRef<HTMLDivElement>(null)
  const { duration } = useMotionTiming(0.85)
  useLayoutEffect(() => { if (status.current) measure(status.current.offsetHeight) }, [measure])
  return <div ref={status} role="status" className={styles.loading}>
    <motion.span aria-hidden="true" className={styles.spinner} animate={duration ? { rotate: 360 } : { rotate: 0 }}
      transition={{ duration, ease: 'linear', repeat: duration ? Infinity : 0 }}>
      <CircleNotchIcon size={17} />
    </motion.span>
    {loading}
  </div>
}

function Loaded({ from, children }: { from?: number; children: ReactNode }) {
  const transition = useMotionTiming(0.22)
  const [expanding, setExpanding] = useState(from !== undefined && transition.duration > 0)
  return <motion.div initial={expanding ? { height: from, opacity: 0 } : false}
    animate={{ height: 'auto', opacity: 1 }} transition={transition} onAnimationComplete={() => setExpanding(false)}
    style={expanding ? { overflow: 'clip' } : undefined}>{children}</motion.div>
}

class LoadBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false }
  static getDerivedStateFromError() { return { failed: true } }
  render() { return this.state.failed ? <LoadError /> : this.props.children }
}

function LoadError() {
  const { loadError, reload } = useRailMessages()
  // A failed module or an outdated hashed chunk needs a fresh document's module map.
  return <div className={styles.loadError}>
    <p role="alert">{loadError}</p>
    <button type="button" className={cx(controls.glass, styles.reload)} onClick={() => window.location.reload()}>{reload}</button>
  </div>
}
