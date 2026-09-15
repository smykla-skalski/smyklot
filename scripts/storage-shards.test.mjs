import { test } from 'node:test'
import assert from 'node:assert/strict'
import { specsFrom, partition, focusFor, verify, timingWeights } from './storage-shards.mjs'

const spec = (name, state = 'passed') => ({ name, state })
const report = (nodes, extra = {}) => [{
  SuiteDescription: 'SQLite Storage Suite', ...extra,
  SpecReports: nodes.map(node => ({
    LeafNodeType: 'It', ContainerHierarchyTexts: ['store [Unit]'],
    LeafNodeText: 'writes', RunTime: 1e9, State: 'passed', ...node,
  })),
}]

test('balances skewed durations without omitting new specs', () => {
  const specs = ['slow', 'medium', 'short', 'new', 'tiny'].map(name => spec(name))
  const weights = { slow: 9, medium: 6, short: 3, tiny: 1 }
  const groups = partition(specs, weights, 3)
  assert.deepEqual(groups.map(group => group.seconds), [9, 9, 7])
  assert.deepEqual(groups.flatMap(group => group.specs.map(item => item.name)).sort(), specs.map(item => item.name).sort())
  assert.deepEqual(partition([...specs].reverse(), weights, 3), groups)
})

test('rejects an empty shard or invalid weights', () => {
  assert.throws(() => partition([spec('one')], { one: 1 }, 2), /Each shard/)
  for (const weights of [{}, { one: 0 }, { one: -1 }, { one: Infinity }, { one: '1' }]) {
    assert.throws(() => partition([spec('one')], weights, 1), /Timing weights/)
  }
})

test('focus matches literal names, not regex metacharacters or partial names', () => {
  const expected = [spec('suite [Unit] writes (a|b) $.name + {x} \\ path?'), spec('other')]
  const regex = new RegExp(focusFor(expected))
  for (const item of expected) assert.ok(regex.test(item.name))
  assert.ok(!regex.test('other suffix'))
  assert.ok(!regex.test('prefix other'))
  assert.ok(!regex.test('suite U writes a $.name + {x} \\ path?'))
})

test('inventory rejects focused, ordered, and serial suites', () => {
  assert.throws(() => specsFrom(report([{}], { SuiteHasProgrammaticFocus: true })), /unfocused/)
  for (const nodes of [[{ IsInOrderedContainer: true }], [{ IsSerial: true }]]) {
    assert.throws(() => specsFrom(report(nodes)), /Cannot shard/)
  }
  assert.deepEqual(specsFrom(report([{}, { LeafNodeType: 'BeforeSuite' }])), [
    { name: 'SQLite Storage Suite store [Unit] writes', state: 'passed', seconds: 1, count: 1 },
  ])
})

test('verifies coverage, rejects skipped, missing, and unexpected execution', () => {
  const expected = [spec('one'), spec('two')]
  verify(expected, [...expected, spec('three', 'skipped')])
  for (const actual of [[], [spec('one')], [spec('one'), spec('two', 'skipped')],
    [...expected, spec('three')], [spec('one'), spec('three')]]) {
    assert.throws(() => verify(expected, actual), /exactly its assigned/)
  }
})

test('keeps duplicate test names together and verifies their multiplicity', () => {
  const duplicate = specsFrom(report([{}, {}]))
  assert.equal(duplicate.length, 1)
  assert.equal(duplicate[0].count, 2)
  assert.equal(duplicate[0].seconds, 2)
  verify(duplicate, duplicate)
  assert.throws(() => verify(duplicate, specsFrom(report([{}]))), /exactly its assigned/)
  assert.equal(specsFrom(report([{}, { State: 'skipped' }]))[0].state, 'mixed')
})

test('rebalancing requires complete, nonduplicated timing coverage', () => {
  const first = report([{ LeafNodeText: 'a' }, { LeafNodeText: 'b', State: 'skipped' }])
  const second = report([{ LeafNodeText: 'a', State: 'skipped' }, { LeafNodeText: 'b' }])
  assert.equal(Object.keys(timingWeights([first, second])).length, 2)
  assert.throws(() => timingWeights([first]), /exactly its assigned/)
  assert.throws(() => timingWeights([first, first, second]), /repeat specs/)
})
