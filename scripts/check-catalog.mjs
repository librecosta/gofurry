import { spawn, spawnSync } from 'node:child_process'
import { randomBytes, randomUUID } from 'node:crypto'
import { appendFile, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { createServer as createHTTPServer, request as proxyRequest } from 'node:http'
import { createServer } from 'node:net'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { setTimeout as delay } from 'node:timers/promises'
import { fileURLToPath } from 'node:url'
import { sitesFixtureSQL } from './sites-fixture.mjs'

const root = fileURLToPath(new URL('../', import.meta.url))
const webDir = join(root, 'apps', 'web')
const scratch = await mkdtemp(join(tmpdir(), 'gofurry-d1-'))
const name = 'gofurry-d1-' + randomUUID()
const processes = []
const edges = []
let container
let redisContainer
let paused = false
let redisPaused = false

function run(command, args, cwd = root, env = process.env, capture = false) {
  const result = spawnSync(command, args, { cwd, env, encoding: 'utf8', stdio: capture ? ['ignore', 'pipe', 'inherit'] : 'inherit' })
  if (result.error || result.status !== 0) throw new Error(`${command} ${args[0]} failed`, { cause: result.error })
  return result.stdout?.trim()
}

function runAsync(command, args, cwd, env) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, { cwd, env, stdio: 'inherit' })
    child.once('error', reject)
    child.once('close', code => code === 0 ? resolve() : reject(new Error(`${command} ${args[0]} failed`)))
  })
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

// The fixture mimics the same-origin edge and replaces browser forwarding headers.
async function fixtureEdge(port, fixture) {
  const edge = createHTTPServer((incoming, response) => {
    const isInteraction = /^\/api\/v1\/sites\/[^/]+\/interactions(?:\?|$)/.test(incoming.url ?? '')
    const base = isInteraction ? fixture.api : fixture.web
    const headers = { ...incoming.headers }
    delete headers.forwarded
    delete headers['x-real-ip']
    delete headers['x-forwarded-for']
    if (isInteraction) headers['x-forwarded-for'] = incoming.socket.remoteAddress
    const upstream = proxyRequest(base, { path: incoming.url, method: incoming.method, headers }, result => {
      response.writeHead(result.statusCode, result.headers)
      result.pipe(response)
    })
    upstream.on('error', () => { if (!response.headersSent) response.writeHead(503); response.end() })
    incoming.pipe(upstream)
  })
  await new Promise((resolve, reject) => edge.once('error', reject).listen(port, '127.0.0.1', resolve))
  edges.push(edge)
}

function yaml(region, databasePort, apiPort) {
  return `server:
  address: 127.0.0.1:${apiPort}
  environment: development
  trusted_proxies: [127.0.0.1]
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
  redisContainer = run('docker', ['run', '--detach', '--name', name + '-redis', '--label', 'gofurry-test=d2-navigation',
    '--publish', '127.0.0.1::6379', 'redis:8.2.1-alpine@sha256:987c376c727652f99625c7d205a1cba3cb2c53b92b0b62aade2bd48ee1593232',
    'redis-server', '--protected-mode', 'no', '--save', '', '--appendonly', 'no'], root, process.env, true)
  const redisAddress = run('docker', ['port', redisContainer, '6379/tcp'], root, process.env, true)
  if (!/^127\.0\.0\.1:\d+$/.test(redisAddress)) throw new Error('Redis fixture must be published on loopback only')
  await waitFor(() => spawnSync('docker', ['exec', redisContainer, 'redis-cli', 'ping'], { stdio: 'ignore' }).status === 0, 'Redis fixture')
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
    const edgePort = await port()
    const config = join(scratch, region + '.yaml')
    await writeFile(config, yaml(region, databasePort, apiPort), { mode: 0o600 })
    fixtures[region] = { apiPort, webPort, edgePort, config, api: 'http://127.0.0.1:' + apiPort,
      web: 'http://127.0.0.1:' + webPort, edge: 'http://127.0.0.1:' + edgePort }
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
  run('docker', ['exec', container, 'createdb', '-U', 'postgres', 'gofurry_d2_navigation_test'])
  const navigationConfig = join(scratch, 'navigation.yaml')
  await writeFile(navigationConfig, yaml('CN', databasePort, await port()).replace('gofurry_d1_cn_test', 'gofurry_d2_navigation_test'), { mode: 0o600 })
  run('go', ['-C', 'services/api', 'test', '-count=1', '-v',
    ...(process.env.NAVIGATION_RACE === '1' ? ['-race'] : []), '-tags=integration', './internal/navigation',
    '-args', '-navigation-config', navigationConfig, '-navigation-redis', redisAddress,
    ...(process.env.NAVIGATION_MEASURE === '1' ? ['-navigation-measure'] : [])])
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
redis:
  address: ${redisAddress}
navigation:
  heat_enabled: ${region === 'CN'}
`)
    if (region === 'CN') await appendFile(fixture.config, `  interaction_origin: ${fixture.edge}
  interaction_hmac_key: ${randomBytes(32).toString('base64url')}
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
      const nav = ['--config', fixture.config, 'navigation']
      const profile = JSON.parse(run(binary, [...nav, 'inspect', '000000000000000a'], root, process.env, true))
      if (profile.rating !== 'sfw') throw new Error('Approved Site fixture lost its review')
      run(binary, [...nav, 'set-feature', 'primary', '0', profile.public_id, '--actor', 'fixture', '--reason', 'Fixture feature'], root, process.env, true)
      await waitFor(async () => (await (await fetch(fixture.api + '/api/v1/sites/discovery')).json()).heat?.status === 'ready',
        'automatic navigation heat')
    }
    launch(process.execPath, ['.output/server/index.mjs'], webDir, {
      ...process.env, NODE_ENV: 'production', HOST: '127.0.0.1', PORT: String(fixture.webPort),
      WEB_REGION: region, WEB_API_BASE_URL: fixture.api, WEB_PUBLIC_ORIGIN: 'https://' + region.toLowerCase() + '.catalog.example',
    })
    await waitFor(() => status(fixture.web + '/readyz', 200), region + ' Web')
    await fixtureEdge(fixture.edgePort, fixture)
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
  await runAsync(process.execPath, ['node_modules/@playwright/test/cli.js', 'test', '--config', 'playwright.catalog.config.ts'], webDir, {
    ...process.env, CATALOG_CN_WEB: fixtures.CN.web, CATALOG_INTL_WEB: fixtures.INTL.web,
    CATALOG_CN_API: fixtures.CN.api, CATALOG_INTL_API: fixtures.INTL.api,
    CATALOG_CN_EDGE: fixtures.CN.edge,
  })

  const apiDetail = fixtures.CN.api + '/api/v1/sites/000000000000000a'
  const webDetail = fixtures.CN.web + '/sites/000000000000000a/example-community'
  const apiYearbook = apiDetail + '/yearbooks/2025'
  const webYearbook = webDetail + '/yearbook/2025'
  async function event(type, { spoofedIP, siteID = '000000000000000a' } = {}) {
    const url = fixtures.CN.edge + '/api/v1/sites/' + siteID + '/interactions'
    const response = await fetch(url, { method: 'POST', signal: AbortSignal.timeout(5000), body: JSON.stringify({ type }),
      headers: { 'Content-Type': 'application/json', Origin: fixtures.CN.edge, 'Sec-Fetch-Site': 'same-origin',
        ...(spoofedIP ? { 'X-Forwarded-For': spoofedIP } : {}) } })
    await response.body?.cancel()
    return response.status
  }
  function totals() {
    return run('docker', ['exec', container, 'psql', '-XAt', '-U', 'postgres', '-d', 'gofurry_d1_cn_test', '-c',
      'SELECT sum(detail_views),sum(outbound_clicks) FROM site_interaction_days'],
    root, process.env, true)
  }
  if (totals() !== '1|0' || await event('detail_view', { spoofedIP: '192.0.2.33' }) !== 202) throw new Error('Edge identity or browser interaction count was wrong')
  run('docker', ['pause', redisContainer])
  redisPaused = true
  // A Redis command can complete after its client times out. Use a different
  // Site for this probe so it cannot reserve the PostgreSQL failure probe.
  if (await event('outbound_click', { siteID: '000000000000000d' }) !== 503 || !await status(fixtures.CN.api + '/api/v1/sites/directory', 200)) {
    throw new Error('Redis outage affected browsing or accepted an event')
  }
  run('docker', ['unpause', redisContainer])
  redisPaused = false
  await waitFor(async () => await event('detail_view') === 202, 'Shared Redis reservations')
  if (totals() !== '1|0') throw new Error('Redis outage wrote an interaction')
  run('docker', ['pause', container])
  paused = true
  if (await event('outbound_click') !== 503) throw new Error('Database outage accepted an event')
  await waitFor(() => status(apiDetail, 503), 'API database-outage status')
  await waitFor(() => status(webDetail, 503), 'SSR database-outage status')
  await waitFor(() => status(apiYearbook, 503), 'Yearbook API database-outage status')
  await waitFor(() => status(webYearbook, 503), 'Yearbook SSR database-outage status')
  await waitFor(() => status(fixtures.CN.api + '/api/v1/sites/directory', 503), 'Navigation database-outage status')
  await waitFor(() => status(fixtures.CN.web + '/sites', 503), 'Discovery SSR database-outage status')
  await waitFor(() => status(fixtures.CN.web + '/sites/directory?category=community', 503), 'Directory SSR database-outage status')
  if (!(await status(fixtures.CN.web + '/start', 200))) throw new Error('Database outage affected Start SSR')
  run('docker', ['unpause', container])
  paused = false
  await waitFor(() => status(apiDetail, 200), 'API database recovery')
  await waitFor(() => status(webDetail, 200), 'SSR database recovery')
  await waitFor(() => status(apiYearbook, 200), 'Yearbook API database recovery')
  await waitFor(() => status(webYearbook, 200), 'Yearbook SSR database recovery')
  await waitFor(() => status(fixtures.CN.api + '/api/v1/sites/directory', 200), 'Navigation database recovery')
  // A request paused in flight may already have reached PostgreSQL before its
  // client deadline. Either commit outcome is valid; replay must add nothing.
  const committed = totals()
  if (!['1|0', '1|1'].includes(committed) || await event('outbound_click') !== 202 || totals() !== committed) {
    throw new Error('An ambiguous aggregate write was incorrectly retried')
  }
  console.log('PostgreSQL outage returned API/SSR 503 for Site and Yearbook, Start remained 200, and both recovered.')
  for (const [region, fixture] of Object.entries(fixtures)) {
    const nav = ['--config', fixture.config, 'navigation']
    const id = '000000000000000a'
    const profile = JSON.parse(run(binary, [...nav, 'inspect', id], root, process.env, true))
    run(binary, [...nav, 'review', id, String(profile.revision), '--rating', region === 'CN' ? 'unreviewed' : 'adult',
      '--actor', 'fixture', '--reason', 'Withdraw public visibility'], root, process.env, true)
    const base = fixture.api + '/api/v1/sites/'
    for (const path of [id, id + '/stats', id + '/yearbooks', id + '/yearbooks/2025']) {
      if (!await status(base + path, 404)) throw new Error(region + ' rating withdrawal leaked through an API')
    }
    const page = '/sites/000000000000000a/example-community'
    for (const path of [page, page + '/yearbook/2025']) {
      if (!await status(fixture.web + path, 404)) throw new Error(region + ' rating withdrawal leaked through SSR')
    }
    const discovery = await (await fetch(base + 'discovery')).json()
    const listing = await (await fetch(base + 'directory')).json()
    if (JSON.stringify(discovery).includes(id) || JSON.stringify(listing).includes(id)) {
      throw new Error(region + ' rating withdrawal leaked through discovery or directory')
    }
  }
  console.log('Navigation: trusted edge dedup, failed-write reservation, Redis/PG outage and both regions’ withdrawal verified.')
  const siteAsset = (kind, file) => JSON.parse(run(binary, [
    '--config', fixtures.CN.config, 'site-assets', 'import', kind,
    join(root, 'services', 'api', 'internal', 'storage', 'testdata', file), '--preflight',
  ], root, process.env, true)).key
  const logo = siteAsset('logo', 'site-logo.webp')
  const cover = siteAsset('cover', 'site-cover.avif')
  for (const region of ['CN', 'INTL']) {
    run('docker', ['exec', container, 'psql', '-Xq', '-v', 'ON_ERROR_STOP=1', '-U', 'postgres',
      '-d', 'gofurry_d1_' + region.toLowerCase() + '_test', '-c', sitesFixtureSQL(region, logo, cover)])
  }
  run(binary, ['--config', fixtures.CN.config, 'navigation', 'refresh-heat'])
  await runAsync(process.execPath, ['node_modules/@playwright/test/cli.js', 'test', '--config', 'playwright.sites.config.ts'], webDir, {
    ...process.env, CATALOG_CN_WEB: fixtures.CN.web, CATALOG_INTL_WEB: fixtures.INTL.web,
    CATALOG_CN_API: fixtures.CN.api, CATALOG_INTL_API: fixtures.INTL.api, CATALOG_CN_EDGE: fixtures.CN.edge,
  })
} finally {
  if (container && paused) run('docker', ['unpause', container])
  if (redisContainer && redisPaused) run('docker', ['unpause', redisContainer])
  for (const edge of edges) { edge.closeAllConnections(); edge.close() }
  for (const { child } of processes.reverse()) child.kill()
  await Promise.all(processes.map(({ exit }) => exit))
  if (container) run('docker', ['rm', '--force', '--volumes', container])
  if (redisContainer) run('docker', ['rm', '--force', '--volumes', redisContainer])
  await rm(scratch, { recursive: true, force: true })
}
