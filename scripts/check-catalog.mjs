import { spawn, spawnSync } from 'node:child_process'
import { randomUUID } from 'node:crypto'
import { appendFile, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { createServer } from 'node:net'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { setTimeout as delay } from 'node:timers/promises'
import { fileURLToPath } from 'node:url'

const root = fileURLToPath(new URL('../', import.meta.url))
const webDir = join(root, 'apps', 'web')
const scratch = await mkdtemp(join(tmpdir(), 'gofurry-d1-'))
const name = 'gofurry-d1-' + randomUUID()
const processes = []
let container
let paused = false

function run(command, args, cwd = root, env = process.env, capture = false) {
  const result = spawnSync(command, args, { cwd, env, encoding: 'utf8', stdio: capture ? ['ignore', 'pipe', 'inherit'] : 'inherit' })
  if (result.error || result.status !== 0) throw new Error(`${command} ${args[0]} failed`, { cause: result.error })
  return result.stdout?.trim()
}

function launch(command, args, cwd, env) {
  const child = spawn(command, args, { cwd, env, stdio: ['ignore', 'inherit', 'inherit'] })
  const exit = new Promise(resolve => { child.once('exit', resolve); child.once('error', resolve) })
  processes.push({ child, exit })
}

async function availablePort() {
  const listener = createServer()
  await new Promise((resolve, reject) => listener.once('error', reject).listen(0, '127.0.0.1', resolve))
  const { port } = listener.address()
  await new Promise((resolve, reject) => listener.close(error => error ? reject(error) : resolve()))
  return port
}

async function waitFor(check, label, timeout = 25000) {
  const end = Date.now() + timeout
  do {
    try { if (await check()) return } catch { /* Service may still be starting. */ }
    await delay(250)
  } while (Date.now() < end)
  throw new Error(label + ' did not become ready')
}

async function status(url, expected) {
  const response = await fetch(url, { signal: AbortSignal.timeout(6500), redirect: 'manual' })
  await response.body?.cancel()
  return response.status === expected
}

function yaml(region, databasePort, apiPort) {
  return `server:
  address: 127.0.0.1:${apiPort}
  environment: development
deployment:
  region: ${region}
logging:
  slow_request: 1s
search_suggestions:
  enabled: false
database:
  host: 127.0.0.1
  port: ${databasePort}
  name: gofurry_d1_${region.toLowerCase()}_test
  user: postgres
  password: ''
  ssl_mode: disable
  connect_timeout: 1s
  max_connections: 5
  min_connections: 0
  max_connection_lifetime: 30m
  max_connection_idle_time: 5m
`
}

try {
  container = run('docker', ['run', '--detach', '--name', name, '--label', 'gofurry-test=d1-catalog',
    '--env', 'POSTGRES_HOST_AUTH_METHOD=trust', '--publish', '127.0.0.1::5432',
    'postgres:18.6@sha256:5a5a84b19854a9ffaa54082c166ff4ec27473a361e496e5ea167f298f2da9722'], root, process.env, true)
  const portText = run('docker', ['port', container, '5432/tcp'], root, process.env, true)
  const databasePort = Number(/^127\.0\.0\.1:(\d+)$/.exec(portText)?.[1])
  if (!databasePort) throw new Error('PostgreSQL fixture must be published on loopback only')
  await waitFor(() => spawnSync('docker', ['exec', container, 'pg_isready', '-h', '127.0.0.1', '-U', 'postgres'], { stdio: 'ignore' }).status === 0, 'PostgreSQL fixture')
  const fixtures = {}
  const used = new Set([databasePort])
  async function port() {
    for (;;) {
      const candidate = await availablePort()
      if (!used.has(candidate)) { used.add(candidate); return candidate }
    }
  }
  for (const region of ['CN', 'INTL']) {
    run('docker', ['exec', container, 'createdb', '-U', 'postgres', 'gofurry_d1_' + region.toLowerCase() + '_test'])
    const apiPort = await port()
    const webPort = await port()
    const config = join(scratch, region + '.yaml')
    await writeFile(config, yaml(region, databasePort, apiPort), { mode: 0o600 })
    fixtures[region] = { apiPort, webPort, config, api: 'http://127.0.0.1:' + apiPort, web: 'http://127.0.0.1:' + webPort }
  }
  run('go', ['-C', 'services/api', 'test', '-count=1', '-v', '-tags=integration', './internal/catalog',
    '-args', '-catalog-cn-config', fixtures.CN.config, '-catalog-intl-config', fixtures.INTL.config])
  run('docker', ['exec', container, 'createdb', '-U', 'postgres', 'gofurry_d1_forward_test'])
  const forwardConfig = join(scratch, 'forward.yaml')
  await writeFile(forwardConfig, yaml('CN', databasePort, await port()).replace('gofurry_d1_cn_test', 'gofurry_d1_forward_test'), { mode: 0o600 })
  run('go', ['-C', 'services/api', 'test', '-count=1', '-v',
    ...(process.env.YEARBOOK_RACE === '1' ? ['-race'] : []), '-tags=integration', './internal/yearbook',
    '-args', '-yearbook-cn-config', fixtures.CN.config, '-yearbook-intl-config', fixtures.INTL.config,
    '-yearbook-forward-config', forwardConfig, ...(process.env.YEARBOOK_MEASURE === '1' ? ['-yearbook-measure'] : [])])
  const binary = join(scratch, process.platform === 'win32' ? 'api.exe' : 'api')
  run('go', ['-C', 'services/api', 'build', '-o', binary, './cmd/api'])

  for (const [region, fixture] of Object.entries(fixtures)) {
    // The existing wallpaper read model uses this base to format URLs. Its
    // PostgreSQL query does not contact object storage.
    await appendFile(fixture.config, `storage:
  endpoint: https://objects.catalog.example
  region: auto
  bucket: fixture
  access_key_id: fixture
  secret_access_key: fixture
  public_base_url: https://wallpapers.catalog.example
yearbook:
  enabled: true
`)
    launch(binary, ['--config', fixture.config], root, process.env)
    await waitFor(() => status(fixture.api + '/readyz', 200), region + ' API')
    await waitFor(() => status(fixture.api + '/api/v1/sites/000000000000000a/yearbooks/2025', 200), region + ' automatic yearbook publication')
    if (region === 'CN') {
      const options = ['--config', fixture.config, 'yearbook']
      const read = args => JSON.parse(run(binary, [...options, ...args], root, process.env, true))
      const preview = read(['preview', '000000000000000a', '2025'])
      const reconcile = read(['reconcile', '--dry-run', '--year', '2025', '--site', '000000000000000a'])
      const audit = read(['audit'])
      if (!preview.eligible || reconcile.digest !== preview.digest || !audit.length || audit.some(row => row.mismatches !== 0)) {
        throw new Error('Yearbook CLI preview/reconcile/audit did not match the fixture')
      }
    }
    launch(process.execPath, ['.output/server/index.mjs'], webDir, {
      ...process.env, NODE_ENV: 'production', HOST: '127.0.0.1', PORT: String(fixture.webPort),
      WEB_REGION: region, WEB_API_BASE_URL: fixture.api, WEB_PUBLIC_ORIGIN: 'https://' + region.toLowerCase() + '.catalog.example',
    })
    await waitFor(() => status(fixture.web + '/readyz', 200), region + ' Web')
  }
  if (process.env.YEARBOOK_MEASURE === '1') {
    const workers = () => run('docker', ['exec', container, 'psql', '-XAt', '-U', 'postgres', '-d', 'postgres', '-c',
      "SELECT datname,state,query_start FROM pg_stat_activity WHERE application_name='gofurry-yearbook' ORDER BY datname"], root, process.env, true)
    await waitFor(() => workers().split('\n').filter(row => row.includes('|idle|')).length === 2, 'Yearbook maintenance connections')
    const before = workers()
    await delay(1500)
    if (workers() !== before) throw new Error('Yearbook maintenance did work between scheduled runs')
    console.log('Yearbook idle sample: one idle maintenance connection per deployment, no SQL activity over 1.5 seconds.')
  }
  run(process.execPath, ['node_modules/@playwright/test/cli.js', 'test', '--config', 'playwright.catalog.config.ts'], webDir, {
    ...process.env, CATALOG_CN_WEB: fixtures.CN.web, CATALOG_INTL_WEB: fixtures.INTL.web,
    CATALOG_CN_API: fixtures.CN.api, CATALOG_INTL_API: fixtures.INTL.api,
  })

  const apiDetail = fixtures.CN.api + '/api/v1/sites/000000000000000a'
  const webDetail = fixtures.CN.web + '/sites/000000000000000a/example-community'
  const apiYearbook = apiDetail + '/yearbooks/2025'
  const webYearbook = webDetail + '/yearbook/2025'
  run('docker', ['pause', container])
  paused = true
  await waitFor(() => status(apiDetail, 503), 'API database-outage status')
  await waitFor(() => status(webDetail, 503), 'SSR database-outage status')
  await waitFor(() => status(apiYearbook, 503), 'Yearbook API database-outage status')
  await waitFor(() => status(webYearbook, 503), 'Yearbook SSR database-outage status')
  if (!(await status(fixtures.CN.web + '/start', 200))) throw new Error('Database outage affected Start SSR')
  run('docker', ['unpause', container])
  paused = false
  await waitFor(() => status(apiDetail, 200), 'API database recovery')
  await waitFor(() => status(webDetail, 200), 'SSR database recovery')
  await waitFor(() => status(apiYearbook, 200), 'Yearbook API database recovery')
  await waitFor(() => status(webYearbook, 200), 'Yearbook SSR database recovery')
  console.log('PostgreSQL outage returned API/SSR 503 for Site and Yearbook, Start remained 200, and both recovered.')
} finally {
  if (container && paused) run('docker', ['unpause', container])
  for (const { child } of processes.reverse()) child.kill()
  await Promise.all(processes.map(({ exit }) => exit))
  if (container) run('docker', ['rm', '--force', '--volumes', container])
  await rm(scratch, { recursive: true, force: true })
}
