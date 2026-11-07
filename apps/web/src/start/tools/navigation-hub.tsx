import { useState } from 'react'
import { GlobeIcon } from '@phosphor-icons/react/dist/ssr/Globe'
import type { Locale } from '../../preferences'
import { usePreferences } from '../../preferences'
import { cx } from '../../styles/classes'
import controls from '../controls.module.scss'
import styles from './tools.module.scss'

export type HubLink = { name: Record<Locale, string>; icon: string; href: `https://${string}` }

export function NavigationHub({ label, links }: { label: string; links: readonly HubLink[] }) {
  const { locale } = usePreferences()
  return <nav aria-label={label} className={styles.navigation}>
    {links.map(({ name, icon, href }) => <a key={href} href={href} target="_blank" rel="noopener noreferrer" data-quick-ignore
      className={cx(controls.glass, styles.destination)}>
      <HubIcon key={icon} src={icon} /><span>{name[locale]}</span>
    </a>)}
  </nav>
}

function HubIcon({ src }: { src: string }) {
  const [failed, setFailed] = useState(false)
  return failed
    ? <GlobeIcon size={24} aria-hidden="true" className={styles.hubIcon} />
    : <img src={src} alt="" width={24} height={24} decoding="async" className={styles.hubIcon} onError={() => setFailed(true)} />
}
