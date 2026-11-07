import styles from './page.module.scss'
import { Outlet } from '@tanstack/react-router'
import { StartBackground } from '../start-background'
import { StartHeader, StartFooter } from './chrome'
import { SearchBox } from './search-box'
import { WallpaperProvider } from './wallpaper-state'
import { WidgetRailsProvider } from './widget-rails-state'
import { SideRails } from './side-rails'

export function StartPage() {
  return <WallpaperProvider><WidgetRailsProvider><div data-start-page className={styles.viewport}>
    <StartBackground />
    <StartHeader />
    <main className={styles.page}><SearchBox /><SideRails /></main>
    <StartFooter />
    <Outlet />
  </div></WidgetRailsProvider></WallpaperProvider>
}
