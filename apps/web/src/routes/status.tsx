import { createFileRoute, Link } from '@tanstack/react-router'
import { useMessages } from '../preferences'
export const Route = createFileRoute('/status')({ component: Status })
function Status() {
  const m = useMessages()
  return <main><h1>Web SSR ready</h1><Link to="/">{m.back}</Link></main>
}
