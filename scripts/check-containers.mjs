import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'

const project = `gofurry-smoke-${process.pid}`
const options = { encoding: 'utf8', env: { ...process.env, WEB_PORT: '0', API_PORT: '0' }, timeout: 60_000 }
const docker = (...args) => execFileSync('docker', args, options).trim()
const compose = (...args) => docker('compose', '-p', project, '-f', 'deploy/compose/compose.yaml', ...args)
try {
  console.log(compose('up', '--no-build', '--wait', '--wait-timeout', '45'))
  for (const [service, port] of [['web', '3000'], ['api', '8080']]) {
    const address = compose('port', service, port)
    const base = `http://${address}`
    for (const path of ['/livez', '/readyz']) assert.equal((await fetch(base + path)).status, 200)
    const res = await fetch(base + (service === 'api' ? '/api/v1/status' : '/'))
    assert.equal(res.status, 200)
    assert.ok(res.headers.get('x-request-id'))
    const id = compose('ps', '-q', service)
    const [container] = JSON.parse(docker('inspect', id))
    assert.equal(container.State.Health.Status, 'healthy')
    assert.deepEqual(container.HostConfig.LogConfig, { Type: 'local', Config: { compress: 'true', 'max-file': '5', 'max-size': '10m' } })
    assert.notEqual(container.Config.User, 'root')
    console.log(`${service}: probes, runtime, non-root user and log rotation OK (${address})`)
  }
  compose('stop', '--timeout', '20')
  for (const service of ['web', 'api']) {
    const id = compose('ps', '-aq', service)
    const [container] = JSON.parse(docker('inspect', id))
    assert.equal(container.State.ExitCode, 0, `${service} must handle SIGTERM`)
    const logs = compose('logs', '--no-log-prefix', service)
    assert.ok(logs.includes('shutdown complete'), `${service} close hook must finish`)
    const entries = logs.split('\n').filter((line) => line.startsWith('{')).map((line) => JSON.parse(line))
    assert.ok(entries.some((entry) => entry.service === `gofurry-${service}` && entry.request_id))
    console.log(`${service}: SIGTERM exited 0, close hook and structured request logs OK`)
  }
} finally {
  compose('down', '--remove-orphans')
}
