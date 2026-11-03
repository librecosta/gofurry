import { createFileRoute } from '@tanstack/react-router'
import * as stylex from '@stylexjs/stylex'
import { StartBackground } from '../start-background'
import { StartHeader, StartFooter } from '../start/chrome'
import { SearchBox } from '../start/search-box'

export const Route = createFileRoute('/start')({
  head: () => ({ meta: [{ title: 'GoFurry · Start' }] }),
  component: Start,
})

function Start() {
  return <>
    <StartBackground />
    <StartHeader />
    <main {...stylex.props(styles.page)}><SearchBox /></main>
    <StartFooter />
  </>
}

const styles = stylex.create({
  page: {
    minHeight: '100dvh', paddingInline: { default: 24, '@media (max-width: 767px)': 16 },
    // Keep the whole group near 27% of the first viewport, with header clearance.
    paddingTop: 'max(84px, calc(27svh - 76px))', paddingBottom: 130,
  },
})
