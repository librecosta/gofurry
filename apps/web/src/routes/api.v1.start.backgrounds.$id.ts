import { createFileRoute } from '@tanstack/react-router'
import { relayBackgrounds } from '../background-relay'
export const Route = createFileRoute('/api/v1/start/backgrounds/$id')({ server: { handlers: { GET: ({ request, params }) => relayBackgrounds(request, params.id) } } })
