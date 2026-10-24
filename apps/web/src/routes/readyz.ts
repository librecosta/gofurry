import { createFileRoute } from '@tanstack/react-router'
export const Route = createFileRoute('/readyz')({
  server: { handlers: { GET: () => Response.json({ status: 'ok' }) } },
})
