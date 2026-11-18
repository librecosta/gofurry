import { readFileSync } from 'node:fs'
import { load } from 'js-yaml'

type WebConfig = { region: 'CN' | 'INTL'; api_origin: string; public_origin?: string }

let config: Readonly<WebConfig> | undefined

function origin(raw: unknown): string {
  if (typeof raw !== 'string' || !/^https?:\/\/[^/?#\\\s]+\/?$/.test(raw)) throw new Error()
  const url = new URL(raw)
  if (!url.hostname || url.username || url.password) throw new Error()
  return url.origin
}

export function webConfig(): Readonly<WebConfig> {
  if (config) return config
  const path = process.env.GOFURRY_WEB_CONFIG_FILE
  if (!path) throw new Error('Set GOFURRY_WEB_CONFIG_FILE to an explicit Web YAML path')
  try {
    const data: unknown = load(readFileSync(path, 'utf8'))
    if (!data || typeof data !== 'object' || Array.isArray(data) ||
        Object.keys(data).some(key => !['region', 'api_origin', 'public_origin'].includes(key))) throw new Error()
    const { region, api_origin, public_origin } = data as Partial<WebConfig>
    if (region !== 'CN' && region !== 'INTL') throw new Error()
    config = Object.freeze({
      region,
      api_origin: origin(api_origin),
      public_origin: public_origin === undefined ? undefined : origin(public_origin),
    })
    return config
  } catch {
    throw new Error('Invalid GoFurry Web YAML; check region and configured origins')
  }
}
