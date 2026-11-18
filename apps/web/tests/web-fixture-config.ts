import { mkdtempSync, rmSync, rmdirSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

export function webFixtureConfig(apiOrigin = 'http://127.0.0.1:17321') {
  const directory = mkdtempSync(join(tmpdir(), 'gofurry-web-fixture-'))
  const path = join(directory, 'config.yaml')
  writeFileSync(path, JSON.stringify({ region: process.env.WEB_REGION ?? 'CN', api_origin: apiOrigin }), { mode: 0o600 })
  process.once('exit', () => rmSync(path, { force: true }))
  process.once('exit', () => { try { rmdirSync(directory) } catch { /* Another fixture process already cleaned up. */ } })
  return path
}
