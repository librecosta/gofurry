import { readdirSync } from 'node:fs'
import { execFileSync } from 'node:child_process'
import { join } from 'node:path'
const walk = (dir) => readdirSync(dir, { withFileTypes: true }).flatMap((f) =>
  f.isDirectory() ? walk(join(dir, f.name)) : f.name.endsWith('.go') ? [join(dir, f.name)] : [])
const goRoot = execFileSync('go', ['env', 'GOROOT'], { encoding: 'utf8' }).trim()
const output = execFileSync(join(goRoot, 'bin', 'gofmt'), ['-l', ...walk('services/api')], { encoding: 'utf8' }).trim()
if (output) throw new Error(`Run gofmt on:\n${output}`)
