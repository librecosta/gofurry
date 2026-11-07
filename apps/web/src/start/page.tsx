import styles from './page.module.scss'
import { Outlet } from '@tanstack/react-router'
import { StartBackground } from '../start-background'
import { usePreferences } from '../preferences'
import { StartHeader, StartFooter } from './chrome'
import { SearchBox } from './search-box'
import { WallpaperProvider } from './wallpaper-state'
import { WidgetRailsProvider } from './widget-rails-state'
import { SideRails } from './side-rails'

export function StartPage() {
  const { startMaterial } = usePreferences()
  return <WallpaperProvider><WidgetRailsProvider><div data-start-page data-start-material={startMaterial} className={styles.viewport}>
    <StartBackground />
    <StartHeader />
    <main className={styles.page}><SearchBox /><SideRails /></main>
    <StartFooter />
    <Outlet />
  </div></WidgetRailsProvider></WallpaperProvider>
}
