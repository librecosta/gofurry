import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { execFileSync } from 'node:child_process'

const read = (path) => readFileSync(new URL(`../${path}`, import.meta.url), 'utf8').trim()
const pkg = JSON.parse(read('package.json'))
assert.equal(process.version, `v${read('.node-version')}`, 'Node version')
assert.equal(pkg.engines.node, read('.node-version'))
const pnpm = pkg.packageManager.split('@')[1]
assert.equal(pkg.engines.pnpm, pnpm)
assert.ok(process.env.npm_config_user_agent?.startsWith(`pnpm/${pnpm} `), 'Run via pinned pnpm')
assert.match(execFileSync('go', ['version'], { encoding: 'utf8' }), new RegExp(`go${read('.go-version').replaceAll('.', '\\.')} `))
assert.ok(execFileSync('task', ['--version'], { encoding: 'utf8' }).includes(read('.task-version')), 'Task version')
assert.ok(read('services/api/go.mod').includes(`go ${read('.go-version')}`))
assert.ok(read('Taskfile.yml').includes(`GOTOOLCHAIN: go${read('.go-version')}`))
assert.ok(read('apps/web/Dockerfile').includes(`node:${read('.node-version')}-`))
assert.ok(read('apps/web/Dockerfile').includes(`pnpm@${pnpm}`))
assert.ok(read('services/api/Dockerfile').includes(`golang:${read('.go-version')}-`))
console.log(`Toolchains OK: Node ${process.version}, pnpm ${pnpm}, Go ${read('.go-version')}, Task ${read('.task-version')}`)
