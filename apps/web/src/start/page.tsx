import styles from './page.module.scss'
import { Outlet } from '@tanstack/react-router'
import { StartBackground } from '../start-background'
import { StartHeader, StartFooter } from './chrome'
import { SearchBox } from './search-box'
import { WallpaperProvider } from './wallpaper-state'

export function StartPage() {
  return <WallpaperProvider><div data-start-page className={styles.viewport}>
    <StartBackground />
    <StartHeader />
    <main className={styles.page}><SearchBox /></main>
    <StartFooter />
    <Outlet />
  </div></WallpaperProvider>
}
