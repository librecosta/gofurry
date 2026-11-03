import * as stylex from '@stylexjs/stylex'
import { Outlet } from '@tanstack/react-router'
import { StartBackground } from '../start-background'
import { StartHeader, StartFooter } from './chrome'
import { SearchBox } from './search-box'

export function StartPage() {
  return <div data-start-page {...stylex.props(styles.viewport)}>
    <StartBackground />
    <StartHeader />
    <main {...stylex.props(styles.page)}><SearchBox /></main>
    <StartFooter />
    <Outlet />
  </div>
}

const styles = stylex.create({
  viewport: { position: 'fixed', inset: 0, height: '100dvh', isolation: 'isolate', overflow: 'clip' },
  page: {
    position: 'relative', zIndex: 1, height: '100%', minWidth: 0, paddingInline: { default: 24, '@media (max-width: 767px)': 16 },
    // Keep the whole group near 27% of the first viewport, with header clearance.
    paddingTop: { default: 'max(84px, calc(27dvh - 76px))', '@media (max-height: 420px)': 68 },
    paddingBottom: 'clamp(36px, 12dvh, 130px)',
  },
})
