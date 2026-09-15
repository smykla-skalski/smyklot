// Partition SQLite's independent specs by recorded duration, longest first.
// New specs get the median weight and are always included in the partition.
import { readFileSync, writeFileSync, mkdirSync, rmSync } from 'node:fs'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { spawnSync } from 'node:child_process'

export const shardCount = 3
export const weightsPath = new URL('./storage-spec-times.json', import.meta.url)

export function specsFrom(report) {
  if (report.length !== 1 || report[0].SuiteHasProgrammaticFocus) {
    throw new Error('Expected one unfocused SQLite suite')
  }
  const suite = report[0]
  const specs = suite.SpecReports.filter(spec => spec.LeafNodeType === 'It')
  const names = new Map()
  for (const spec of specs) {
    const name = [suite.SuiteDescription, ...spec.ContainerHierarchyTexts, spec.LeafNodeText].join(' ')
    if (spec.IsInOrderedContainer || spec.IsSerial) {
      throw new Error(`Cannot shard ordered or serial spec: ${name}`)
    }
    const previous = names.get(name)
    names.set(name, {
      name,
      state: previous && previous.state !== spec.State ? 'mixed' : spec.State,
      seconds: (previous?.seconds ?? 0) + spec.RunTime / 1e9,
      count: (previous?.count ?? 0) + 1,
    })
  }
  return [...names.values()]
}

export function partition(specs, weights, count = shardCount) {
  if (!Number.isInteger(count) || count < 1 || specs.length < count) {
    throw new Error('Each shard must contain specs')
  }
  const recorded = Object.values(weights).sort((a, b) => a - b)
  if (!recorded.length || recorded.some(value => !Number.isFinite(value) || value <= 0)) {
    throw new Error('Timing weights must be positive finite numbers')
  }
  const fallback = recorded[Math.floor(recorded.length / 2)]
  const weight = spec => weights[spec.name] ?? fallback
  const compare = (a, b) => a.name < b.name ? -1 : a.name > b.name ? 1 : 0
  const sorted = [...specs].sort((a, b) => weight(b) - weight(a) || compare(a, b))
  const groups = Array.from({ length: count }, () => ({ specs: [], seconds: 0 }))
  for (const spec of sorted) {
    const group = groups.reduce((best, next) => next.seconds < best.seconds ? next : best)
    group.specs.push(spec)
    group.seconds += weight(spec)
  }
  if (groups.some(group => !group.specs.length)) throw new Error('Empty shard')
  return groups
}

export function focusFor(specs) {
  return `^(?:${specs.map(spec => spec.name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('|')})$`
}

export function verify(expected, actual) {
  const selected = new Set(expected.map(spec => spec.name))
  const passed = actual.filter(spec => spec.state === 'passed')
  if (passed.length !== selected.size || passed.some(spec => !selected.has(spec.name) ||
      (spec.count ?? 1) !== (expected.find(item => item.name === spec.name)?.count ?? 1)) ||
      actual.some(spec => selected.has(spec.name) && spec.state !== 'passed')) {
    throw new Error('Shard did not pass exactly its assigned specs')
  }
}

function run(args, report) {
  rmSync(report, { force: true })
  const result = spawnSync('ginkgo', [
    '--race', '--fail-on-empty', `--json-report=${report}`, ...args,
    './internal/storage/sqlite', '--', '-test.run=^TestSQLite$',
  ], { stdio: 'inherit' })
  if (result.error) throw result.error
  if (result.status !== 0) throw new Error(`Ginkgo failed (${result.status ?? result.signal})`)
  return specsFrom(JSON.parse(readFileSync(report, 'utf8')))
}

export function timingWeights(reports) {
  const inventory = specsFrom(reports[0])
  const passed = reports.flatMap(specsFrom).filter(spec => spec.state === 'passed')
  if (new Set(passed.map(spec => spec.name)).size !== passed.length) {
    throw new Error('Timing reports repeat specs')
  }
  verify(inventory, passed)
  return Object.fromEntries(passed.sort((a, b) => a.name < b.name ? -1 : 1)
    .map(spec => [spec.name, Math.max(0.001, Number(spec.seconds.toFixed(3)))]))
}

function main() {
  if (process.argv[2] === '--update-timings') {
    const files = process.argv.slice(3)
    if (!files.length) throw new Error('Pass a full profile or every shard report')
    const reports = files.map(file => JSON.parse(readFileSync(file, 'utf8')))
    writeFileSync(weightsPath, `${JSON.stringify(timingWeights(reports), null, 2)}\n`)
    return
  }

  const shard = Number(process.argv[2])
  if (!Number.isInteger(shard) || shard < 1 || shard > shardCount) {
    throw new Error(`Pass a shard number from 1 to ${shardCount}`)
  }
  const output = resolve(`tmp/storage/shard-${shard}`)
  mkdirSync(output, { recursive: true })
  const inventory = run(['--dry-run'], `${output}/inventory.json`)
  if (inventory.some(spec => spec.state !== 'passed')) throw new Error('Inventory contains disabled specs')
  const weights = JSON.parse(readFileSync(weightsPath, 'utf8'))
  const groups = partition(inventory, weights)
  for (const [index, group] of groups.entries()) {
    console.log(`Shard ${index + 1}: ${group.specs.reduce((sum, spec) => sum + spec.count, 0)} specs, ${group.seconds.toFixed(1)} weighted seconds`)
  }
  const selected = groups[shard - 1].specs
  const actual = run(['-p', `--focus=${focusFor(selected)}`], `${output}/results.json`)
  verify(selected, actual)
  console.log(`Verified ${selected.reduce((sum, spec) => sum + spec.count, 0)} assigned specs passed on shard ${shard}`)
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) main()
