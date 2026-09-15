import { readFileSync, writeFileSync } from 'node:fs'
import { spawnSync } from 'node:child_process'
import { executeShard, timingWeights } from './storage-shards.mjs'

const weightsPath = new URL('./service-spec-times.json', import.meta.url)
const [engine, rawShard] = process.argv.slice(2)
if (engine === '--update-timings') {
  const files = process.argv.slice(3)
  if (!files.length) throw new Error('Pass a complete service profile or both shard reports')
  const reports = files.map(file => JSON.parse(readFileSync(file, 'utf8')))
  writeFileSync(weightsPath, `${JSON.stringify(timingWeights(reports), null, 2)}\n`)
  process.exit(0)
}
if (!['sqlite', 'postgres'].includes(engine)) throw new Error('Choose sqlite or postgres')
if (engine === 'postgres' && !process.env.SMYKLOT_TEST_POSTGRES_DSN) {
  throw new Error('SMYKLOT_TEST_POSTGRES_DSN is required for the PostgreSQL pass')
}
if (engine === 'sqlite' && process.env.SMYKLOT_TEST_POSTGRES_DSN) {
  throw new Error('Unset SMYKLOT_TEST_POSTGRES_DSN for the SQLite pass')
}
const shard = Number(rawShard)
executeShard({
  // CI's ordinary tests cost 60-81s versus 275-386s of conformance (22%).
  // Reserve that work on shard 1 before balancing the independent specs.
  count: 2, firstShardOverhead: 0.22, packagePath: './cmd/smyklot', bootstrap: 'TestMain',
  weightsPath,
  outputRoot: `tmp/service-${engine}`,
}, shard)

// Ginkgo workers also run ordinary Go tests. Execute those once, on shard 1.
if (shard === 1) {
  const result = spawnSync('go', ['test', '-race', '-count=1', '-skip=^TestMain$', './cmd/smyklot'], { stdio: 'inherit' })
  if (result.error) throw result.error
  if (result.status !== 0) throw new Error(`Service support tests failed (${result.status ?? result.signal})`)
}
