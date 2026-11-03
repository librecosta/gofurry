import styles from './page.module.scss'
import { Outlet } from '@tanstack/react-router'
import { StartBackground } from '../start-background'
import { StartHeader, StartFooter } from './chrome'
import { SearchBox } from './search-box'

export function StartPage() {
  return <div data-start-page className={styles.viewport}>
    <StartBackground />
    <StartHeader />
    <main className={styles.page}><SearchBox /></main>
    <StartFooter />
    <Outlet />
  </div>
}
