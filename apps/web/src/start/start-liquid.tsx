import { useLayoutEffect } from 'react'
import { QUALITY_PRESETS, supported } from '@librecosta/liquid-glass'
import { useLiquidGlass } from '@librecosta/liquid-glass/react'
import './start-liquid.scss'

type BindingProps = { node: HTMLElement; onReady: (node: HTMLElement | null) => void }

export default function LiquidBinding(props: BindingProps) {
  return supported() ? <BackdropBinding {...props} /> : null
}

function BackdropBinding({ node, onReady }: BindingProps) {
  const { ref } = useLiquidGlass<HTMLElement>({ options: QUALITY_PRESETS.Fast })

  useLayoutEffect(() => {
    // The hook attaches on its next commit. Cached maps also write the URL,
    // so readiness must not depend on the library's cache-miss callback.
    const observer = new MutationObserver(() => {
      if (!node.style.getPropertyValue('--hyalite').startsWith('url(')) return
      observer.disconnect()
      onReady(node)
    })
    observer.observe(node, { attributes: true, attributeFilter: ['style'] })
    ref(node)
    return () => {
      observer.disconnect()
      ref(null)
      onReady(null)
    }
  }, [node, onReady, ref])

  return null
}
