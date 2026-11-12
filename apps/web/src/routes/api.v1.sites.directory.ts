import { createFileRoute } from '@tanstack/react-router'
import { relayDirectory } from '../navigation/read.server'

export const Route = createFileRoute('/api/v1/sites/directory')({
  server: { handlers: { GET: ({ request }) => relayDirectory(request) } },
})
