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
import { profileFixtureSQL, profileID } from './site-profile-fixture.mjs'
import { collectionGame, gamesFixtureSQL, memberTwo } from './game-channel-fixture.mjs'

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

async function observedAPI(port, fixture) {
  const listener = createHTTPServer((incoming, response) => {
    fixture.ssrPaths?.push(incoming.url?.split('?')[0])
    const headers = { ...incoming.headers }
    if (fixture.ssrClient) headers['x-forwarded-for'] = fixture.ssrClient
    const upstream = proxyRequest(fixture.api, { path: incoming.url, method: incoming.method, headers }, result => {
      response.writeHead(result.statusCode, result.headers)
      result.pipe(response)
    })
    upstream.on('error', () => { if (!response.headersSent) response.writeHead(503); response.end() })
    response.on('close', () => upstream.destroy())
    incoming.pipe(upstream)
  })
  await new Promise((resolve, reject) => listener.once('error', reject).listen(port, '127.0.0.1', resolve))
  edges.push(listener)
  return 'http://127.0.0.1:' + port
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
    run('docker', ['exec', container, 'psql', '-Xq', '-v', 'ON_ERROR_STOP=1', '-U', 'postgres',
      '-d', 'gofurry_d1_' + region.toLowerCase() + '_test', '-c', gamesFixtureSQL(region)])
    const editor = join(scratch, region + '-collection.json')
    const members = join(scratch, region + '-members.json')
    await writeFile(editor, JSON.stringify({ slug: 'fox-volumes', localizations: [
      { locale: 'en', name: 'Fox Volumes', summary: 'An editorial collection of fox stories.' },
      ...(region === 'CN' ? [{ locale: 'zh', name: '狐狸故事集', summary: '一次策展。' }] : []),
    ] }))
    await writeFile(members, JSON.stringify([memberTwo, collectionGame, '2000000000000029']))
    const channel = (args, capture = true) => run(binary, ['--config', fixture.config, 'game-channel', ...args], root, process.env, capture)
    const created = JSON.parse(channel(['create', editor]))
    const numbered = JSON.parse(channel(['members', created.public_id, String(created.revision), members]))
    const published = JSON.parse(channel(['status', created.public_id, String(numbered.revision), 'published']))
    fixture.gameCollection = published.public_id
    if (region === 'CN') {
      channel(['feature', 'primary', '1', collectionGame])
      channel(['feature', 'collection_1', '1', published.public_id])
      channel(['feature', 'secondary_1', '1', '2000000000000029'])
      const conflict = spawnSync(binary, ['--config', fixture.config, 'game-channel', 'members', created.public_id, '1', members], { cwd: root, encoding: 'utf8' })
      if (conflict.status !== 1 || !conflict.stderr.includes('revision or source conflict')) throw new Error('Stale collection edit was accepted')
    }
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
      WEB_REGION: region, WEB_API_BASE_URL: await observedAPI(await port(), fixture), WEB_PUBLIC_ORIGIN: 'https://' + region.toLowerCase() + '.catalog.example',
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
    GAME_COLLECTION_CN: fixtures.CN.gameCollection, GAME_COLLECTION_INTL: fixtures.INTL.gameCollection,
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
  if (!(await status(fixtures.CN.api + '/api/v1/games/directory', 200))) throw new Error('Redis outage affected the game directory')
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
  await waitFor(() => status(fixtures.CN.web + '/games/directory', 503), 'Game directory SSR database-outage status')
  await waitFor(() => status(fixtures.CN.web + '/games/collections/' + fixtures.CN.gameCollection + '/fox-volumes', 503), 'Collection SSR database-outage status')
  if (!(await status(fixtures.CN.web + '/start', 200))) throw new Error('Database outage affected Start SSR')
  run('docker', ['unpause', container])
  paused = false
  await waitFor(() => status(apiDetail, 200), 'API database recovery')
  await waitFor(() => status(webDetail, 200), 'SSR database recovery')
  await waitFor(() => status(apiYearbook, 200), 'Yearbook API database recovery')
  await waitFor(() => status(webYearbook, 200), 'Yearbook SSR database recovery')
  await waitFor(() => status(fixtures.CN.api + '/api/v1/sites/directory', 200), 'Navigation database recovery')
  await waitFor(() => status(fixtures.CN.web + '/games', 200), 'Game discovery database recovery')
  // A request paused in flight may already have reached PostgreSQL before its
  // client deadline. Either commit outcome is valid; replay must add nothing.
  const committed = totals()
  if (!['1|0', '1|1'].includes(committed) || await event('outbound_click') !== 202 || totals() !== committed) {
    throw new Error('An ambiguous aggregate write was incorrectly retried')
  }
  console.log('PostgreSQL outage returned API/SSR 503 for Site and Yearbook, Start remained 200, and both recovered.')
  const fixtureSQL = (region, sql) => run('docker', ['exec', container, 'psql', '-Xq', '-v', 'ON_ERROR_STOP=1', '-U', 'postgres',
    '-d', 'gofurry_d1_' + region.toLowerCase() + '_test', '-c', sql])
  async function catalogProjection(region, fixture) {
    const categorized = '000000000000000a'
    const uncategorized = '000000000000000j'
    const game = '000000000000000g'
    const api = fixture.api + '/api/v1'
    const read = async path => {
      const response = await fetch(api + path, { headers: { 'X-Forwarded-For': fixture.ssrClient } })
      if (!response.ok) throw new Error(region + ' ' + path + ': ' + response.status)
      return response.json()
    }
    const [site, unclassified, games, sites, directory, annual] = await Promise.all([
      read('/sites/' + categorized), read('/sites/' + uncategorized), read('/games'),
      read('/sites'), read('/sites/directory?sort=heat_7d'), read('/sites/' + categorized + '/yearbooks/2025'),
    ])
    for (const id of [categorized, uncategorized]) {
      if (![sites, directory].every(page => page.items.some(row => row.public_id === id))) throw new Error('Site missing from catalog')
    }
    const listing = games.items.find(row => row.public_id === game)
    if (!listing || !site.primary_category || unclassified.primary_category) throw new Error('Catalog taxonomy or Game identity changed')
    const presentation = await read('/games/' + game)
    const english = region === 'CN' ? '/en' : ''
    const chinese = region === 'CN' ? '' : '/zh'
    const details = [
      [site, 'sites', english, 'Example Community', 'A curated English summary.'],
      [unclassified, 'sites', chinese, '无类手册', '双语索引。'],
      [presentation, 'games', english, 'Fixture Starlight', 'Collected English synopsis.'],
    ]
    for (const [row, kind, prefix, name, summary] of details) {
      const path = prefix + '/' + kind + '/' + row.public_id + '/' + row.slug
      const response = await fetch(fixture.web + path, { redirect: 'manual' })
      const html = await response.text()
      const canonical = 'https://' + region.toLowerCase() + '.catalog.example' + path
      if (response.status !== 200 || !html.includes('<title>' + name + '</title>') ||
          !html.includes('content="' + summary + '"') || !html.includes('rel="canonical" href="' + canonical + '"')) {
        throw new Error('Time-forward SSR changed identity, locale or content at ' + path)
      }
    }
    if (!await status(fixture.web + english + '/sites/' + categorized + '/example-community/yearbook/2025', 200)) {
      throw new Error('Published yearbook lost its URL')
    }
    return { directory, stable: [site, unclassified, listing, presentation.localizations, annual] }
  }
  for (const [region, fixture] of Object.entries(fixtures)) {
    // A separate simulated client keeps the time-forward batch and browser
    // suite within their normal per-client catalog budgets.
    fixture.ssrClient = '192.0.2.43'
    const baseline = await catalogProjection(region, fixture)
    for (const days of [31, 181, 365]) {
      const age = "now()-interval '" + days + " days'"
      fixtureSQL(region, "UPDATE sites SET published_at=" + age + ", first_published_at=" + age +
        " WHERE public_id IN ('000000000000000a','000000000000000j'); UPDATE games SET published_at=" + age +
        " WHERE public_id='000000000000000g'; UPDATE game_intake_candidates SET profile_at=" + age +
        ",review_until=" + age + " WHERE external_key='40001'; UPDATE site_navigation_heat_state SET computed_at=" + age)
      const result = await catalogProjection(region, fixture)
      if (JSON.stringify(result.stable) !== JSON.stringify(baseline.stable) || result.directory.effective_sort === 'heat_7d') {
        throw new Error(region + ' published content or Heat fallback changed at day ' + days)
      }
    }
    fixtureSQL(region, "UPDATE game_sources SET active=false WHERE external_key='40001'; DELETE FROM game_intake_candidates WHERE external_key='40001'")
    const after = await catalogProjection(region, fixture)
    if (JSON.stringify(after.stable) !== JSON.stringify(baseline.stable)) throw new Error('Game depended on an active source or Candidate')
    fixture.ssrClient = undefined
  }
  console.log('R1: CN/INTL catalog, SSR, canonical URLs and published yearbook survive 31/181/365 days; Game survives disabled source and removed Candidate.')
  fixtureSQL('CN', "UPDATE sites SET content_rating='unreviewed',legacy_detail=true WHERE public_id='000000000000000a'")
  const legacy = await fetch(webDetail)
  if (legacy.status !== 200 || legacy.headers.get('x-robots-tag') !== 'noindex' ||
      !(await legacy.text()).includes('/example-community/yearbook/2025') || !await status(apiYearbook, 200)) {
    throw new Error('Legacy compatibility lost its published yearbook or noindex status')
  }
  fixtureSQL('CN', "UPDATE sites SET content_rating='sfw',legacy_detail=false WHERE public_id='000000000000000a'")
  for (const [region, fixture] of Object.entries(fixtures)) {
    const nav = ['--config', fixture.config, 'navigation']
    const id = '000000000000000a'
    const profile = JSON.parse(run(binary, [...nav, 'inspect', id], root, process.env, true))
    const withdrawn = JSON.parse(run(binary, [...nav, 'review', id, String(profile.revision), '--rating', 'adult', '--keep-taxonomy',
      '--actor', 'fixture', '--reason', 'Withdraw public visibility'], root, process.env, true))
    const base = fixture.api + '/api/v1/sites/'
    for (const path of [id, id + '/stats', id + '/related', id + '/yearbooks', id + '/yearbooks/2025']) {
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
    run(binary, [...nav, 'review', id, String(withdrawn.revision), '--rating', 'sfw', '--keep-taxonomy',
      '--actor', 'fixture', '--reason', 'Reverse fixture policy disposition'], root, process.env, true)
    if (!await status(fixture.web + page, 200) || !await status(base + id + '/yearbooks/2025', 200)) {
      throw new Error(region + ' explicit reversal lost the original identity or yearbook')
    }
    if (region === 'CN' && (await (await fetch(base + 'discovery')).json()).featured.primary?.public_id !== id) {
      throw new Error('Rating disposition erased the editorial assignment')
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
    fixtureSQL(region, sitesFixtureSQL(region, logo, cover) + profileFixtureSQL(region))
  }
  const cn = fixtures.CN
  const profileURL = cn.api + '/api/v1/sites/' + profileID
  const readProfile = async () => {
    const response = await fetch(profileURL, { headers: { 'X-Forwarded-For': '192.0.2.44' } })
    if (!response.ok) throw new Error('Site profile returned ' + response.status)
    return response.json()
  }
  const before = await readProfile()
  const options = ['--config', cn.config]
  const op = ['site-profile', 'set-description', profileID]
  const revision = JSON.parse(run(binary, [...options, 'navigation', 'inspect', profileID], root, process.env, true)).revision
  const enText = join(scratch, 'description.en.txt')
  const zhText = join(scratch, 'description.zh.txt')
  await writeFile(enText, 'Pinewood Commons is a meeting place for artists, makers, and friends. Its project forum makes room for works in progress and thoughtful feedback.\n\nMembers share event plans, technique notes, and invitations to collaborate on costumes and stories.', { mode: 0o600 })
  await writeFile(zhText, '松林兽圈是创作者、手工爱好者和朋友们的交流空间。项目论坛欢迎大家分享未完成的作品，交流具体的修改建议。\n\n成员会分享活动计划、制作笔记，也会邀请同好一起完成兽装和故事。', { mode: 0o600 })
  const en = JSON.parse(run(binary, [...options, ...op, 'en', String(revision), enText, '--confirm-reviewed'], root, process.env, true))
  const rejected = args => spawnSync(binary, [...options, ...op, ...args], { cwd: root, stdio: 'ignore' }).status === 1
  if (!rejected(['zh', String(revision), zhText, '--confirm-reviewed']) || !rejected(['zh', String(en.revision), zhText])) {
    throw new Error('Site profile CLI accepted a stale revision or missing review confirmation')
  }
  const zh = JSON.parse(run(binary, [...options, ...op, 'zh', String(en.revision), zhText, '--confirm-reviewed'], root, process.env, true))
  const after = await readProfile()
  if (zh.revision !== revision + 2 || after.localizations.filter(x => x.body?.includes('\n\n')).length !== 2 ||
      ['addresses', 'tags', 'primary_category', 'logo_url'].some(key => JSON.stringify(before[key]) !== JSON.stringify(after[key]))) {
    throw new Error('Site profile CLI changed unrelated fields or lost one language')
  }
  cn.ssrPaths = []
  const begin = performance.now()
  const ssr = await fetch(cn.web + '/en/sites/' + profileID + '/field-guide-1')
  const bytes = (await ssr.arrayBuffer()).byteLength
  const actual = cn.ssrPaths
  cn.ssrPaths = undefined
  if (!ssr.ok || actual.length !== 1 || actual[0] !== '/api/v1/sites/' + profileID) throw new Error('Unexpected detail SSR API fan-out: ' + actual)
  console.log(`Site profile: ${bytes} B SSR HTML, ${(performance.now() - begin).toFixed(1)} ms local, 1 regional API read; bilingual CLI CAS verified.`)
  run(binary, ['--config', fixtures.CN.config, 'navigation', 'refresh-heat'])
  cn.ssrClient = '192.0.2.45'
  await runAsync(process.execPath, ['node_modules/@playwright/test/cli.js', 'test', '--config', 'playwright.sites.config.ts'], webDir, {
    ...process.env, CATALOG_CN_WEB: fixtures.CN.web, CATALOG_INTL_WEB: fixtures.INTL.web,
    CATALOG_CN_API: fixtures.CN.api, CATALOG_INTL_API: fixtures.INTL.api, CATALOG_CN_EDGE: fixtures.CN.edge,
  })
  cn.ssrClient = '192.0.2.44'
  const evidence = " FROM collection_sources s WHERE c.source_id=s.id AND s.namespace='profile-fixture'"
  fixtureSQL('CN', "UPDATE collection_current c SET ruleset='web-passive-2'" + evidence + " AND c.facts ? 'tech.detected.astro'")
  const revised = (await readProfile()).technology
  if (!revised || revised.length !== 2 || revised.some(fact => fact.technology)) throw new Error('Unknown detector ruleset reached the public profile')
  fixtureSQL('CN', "UPDATE collection_bindings SET effective_from=now()-interval '400 days' WHERE source_id IN " +
    "(SELECT id FROM collection_sources WHERE namespace='profile-fixture'); UPDATE collection_plan_bindings SET attached_at=now()-interval '400 days'" +
    " WHERE source_id IN (SELECT id FROM collection_sources WHERE namespace='profile-fixture')")
  fixtureSQL('CN', "UPDATE collection_current c SET state='withdrawn',observation_id=NULL" + evidence + " AND c.facts ? 'tls.certificate'")
  for (const days of [31, 181, 365]) {
    fixtureSQL('CN', "UPDATE collection_current c SET observed_at=current_date-interval '" + days + " days',confirmed_at=current_date-interval '" + days +
      " days'" + evidence + " AND c.facts ? 'http.response'")
    const historical = (await readProfile()).technology
    const fact = historical?.[0]
    if (historical?.length !== 1 || fact?.kind !== 'http' || fact.observation_state !== 'last_observed' ||
        Math.abs((Date.now() - Date.parse(fact.last_observed_at)) / 86400000 - days) >= 2 ||
        fact.source !== 'field-guide.example' || fact.http_status !== 200) throw new Error('Historical evidence lost attribution or claimed freshness')
  }
  fixtureSQL('CN', "SELECT save_site_technology(generation,NULL,NULL) FROM collection_consumer WHERE namespace='profile-fixture'; " +
    "DELETE FROM collection_current c USING collection_sources s WHERE c.source_id=s.id AND s.namespace='profile-fixture'")
  const retained = (await readProfile()).technology
  if (retained?.length !== 1 || retained[0].observation_state !== 'last_observed' || retained[0].kind !== 'http') {
    throw new Error('Pruning the live projection erased historical evidence or resurrected withdrawn evidence')
  }
  const historicalHTML = await (await fetch(cn.web + '/en/sites/' + profileID + '/field-guide-1')).text()
  if (!historicalHTML.includes('Last observed')) throw new Error('Site SSR failed to label old evidence')
  console.log('Site profile: attributed 31/181/365-day HTTP history; unknown detector rules and withdrawn TLS omitted.')
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
