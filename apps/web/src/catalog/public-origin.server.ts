import { getRequestUrl } from '@tanstack/react-start/server'

export function publicOrigin() {
  const configured = process.env.WEB_PUBLIC_ORIGIN
  const url = configured ? new URL(configured) : getRequestUrl({ xForwardedHost: false, xForwardedProto: false })
  if (url.username || url.password || !['http:', 'https:'].includes(url.protocol)) throw new Error('Invalid public origin')
  if (configured ? (url.pathname !== '/' || url.search || url.hash) : !['127.0.0.1', '[::1]', 'localhost'].includes(url.hostname)) {
    throw new Error('An explicit WEB_PUBLIC_ORIGIN is required')
  }
  return url.origin
}
