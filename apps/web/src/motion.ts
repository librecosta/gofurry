import { useReducedMotion } from 'motion/react'

export function useMotionTiming(seconds = 0.42) {
  const reduced = useReducedMotion()
  return { duration: reduced ? 0 : seconds, ease: [0.22, 1, 0.36, 1] as const }
}
