import { execFileSync } from 'node:child_process'
import { readlinkSync } from 'node:fs'
import { setTimeout } from 'node:timers/promises'
import { fileURLToPath } from 'node:url'

const root = fileURLToPath(new URL('../', import.meta.url))
const normalize = value => process.platform === 'win32' ? value.replaceAll('\\', '/').toLowerCase() : value
const checkout = normalize(root).replace(/\/$/, '') + '/'
const ports = { web: 17320, api: 17321 }
const target = process.argv[2]
if (target && !Object.hasOwn(ports, target)) throw new Error('Use stop-dev.mjs [web|api]')
const selected = target ? [[target, ports[target]]] : Object.entries(ports)
const run = (file, args) => execFileSync(file, args, { encoding: 'utf8', windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'] }).trim()

function listeners(port) {
  if (process.platform === 'win32') {
    // Numeric ports are internal constants; no paths or user input enter shell code.
    const output = run('powershell.exe', ['-NoProfile', '-NonInteractive', '-Command', `
      [Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
      $ErrorActionPreference = 'Stop'
      $items = @(Get-NetTCPConnection -State Listen -ErrorAction Stop | Where-Object LocalPort -eq ${port} | ForEach-Object {
        $serviceProcess = Get-CimInstance Win32_Process -Filter ('ProcessId = ' + $_.OwningProcess)
        [PSCustomObject]@{ pid = $_.OwningProcess; command = $serviceProcess.CommandLine; executable = $serviceProcess.ExecutablePath }
      })
      ConvertTo-Json -InputObject $items -Compress
    `])
    return JSON.parse(output)
  }
  let ids
  try { ids = run('lsof', ['-nP', `-iTCP:${port}`, '-sTCP:LISTEN', '-t']) }
  catch (error) { if (error.status === 1) return []; throw new Error('Stopping local services requires lsof on macOS/Linux.', { cause: error }) }
  return [...new Set(ids.split(/\s+/).map(Number))].map(pid => {
    const command = run('ps', ['-ww', '-p', String(pid), '-o', 'command='])
    let cwd = ''
    try { cwd = process.platform === 'linux' ? readlinkSync(`/proc/${pid}/cwd`) : run('lsof', ['-a', '-p', String(pid), '-d', 'cwd', '-Fn']).split('\n').find(line => line.startsWith('n'))?.slice(1) ?? '' } catch { /* Command ownership is still checked. */ }
    return { pid, command, cwd }
  })
}

for (const [name, port] of selected) {
  const processes = [...new Map(listeners(port).map(item => [item.pid, item])).values()]
  const stopped = new Set()
  if (!processes.length) { console.log(`${name}: port ${port} is already free.`); continue }
  for (const item of processes) {
    const owned = [item.command, item.executable].some(value => typeof value === 'string' && normalize(value).includes(checkout)) || normalize(item.cwd ?? '') + '/' === checkout
    if (!owned || item.pid === process.pid) {
      console.error(`${name}: refusing to stop PID ${item.pid} on port ${port}; current checkout ownership is unverified.`)
      process.exitCode = 1
      continue
    }
    try { process.kill(item.pid, 'SIGTERM'); stopped.add(item.pid) }
    catch (error) { if (error.code !== 'ESRCH') throw error }
  }
  if (stopped.size) {
    const deadline = Date.now() + 15000
    while (listeners(port).some(item => stopped.has(item.pid))) {
      if (Date.now() >= deadline) throw new Error(`${name}: port ${port} is still stopping; wait for shutdown before restarting.`)
      await setTimeout(200)
    }
    console.log(`${name}: stopped ${stopped.size} process(es); port ${port} released.`)
  }
}
