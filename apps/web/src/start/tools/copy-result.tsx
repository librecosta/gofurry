import { useState } from 'react'
import { CheckIcon } from '@phosphor-icons/react/dist/ssr/Check'
import { CopyIcon } from '@phosphor-icons/react/dist/ssr/Copy'
import { WarningCircleIcon } from '@phosphor-icons/react/dist/ssr/WarningCircle'
import { usePreferences } from '../../preferences'
import { cx } from '../../styles/classes'
import controls from '../controls.module.scss'
import styles from './tools.module.scss'

export function CopyResult({ value }: { value: string }) {
  const { locale } = usePreferences()
  const [status, setStatus] = useState<'copied' | 'failed'>()
  const label = locale === 'zh' ? '复制结果' : 'Copy result'
  const message = status === 'copied' ? (locale === 'zh' ? '已复制' : 'Copied')
    : status === 'failed' ? (locale === 'zh' ? '复制失败' : 'Could not copy') : ''
  async function copy() {
    try { await navigator.clipboard.writeText(value); setStatus('copied') } catch { setStatus('failed') }
  }
  const Icon = status === 'copied' ? CheckIcon : status === 'failed' ? WarningCircleIcon : CopyIcon
  return <>
    <button type="button" aria-label={label} className={cx(controls.glass, styles.copy)} disabled={!value} onClick={() => void copy()}>
      <Icon size={17} aria-hidden="true" />
    </button>
    <span role="status" className={styles.copyStatus}>{message}</span>
  </>
}
