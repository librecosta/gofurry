import { createFileRoute, Link } from '@tanstack/react-router'

export const Route = createFileRoute('/status')({
  component: () => <main><h1>Web SSR ready</h1><Link to="/">返回</Link></main>,
})
