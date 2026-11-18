import { webConfig } from '../web-config.server'
import { getRequestUrl } from '@tanstack/react-start/server'

export function publicOrigin() {
  const configured = webConfig().public_origin
  const url = configured ? new URL(configured) : getRequestUrl({ xForwardedHost: false, xForwardedProto: false })
  if (url.username || url.password || !['http:', 'https:'].includes(url.protocol)) throw new Error('Invalid public origin')
  if (configured ? (url.pathname !== '/' || url.search || url.hash) : !['127.0.0.1', '[::1]', 'localhost'].includes(url.hostname)) {
    throw new Error('An explicit public_origin is required in Web YAML')
  }
  return url.origin
}
