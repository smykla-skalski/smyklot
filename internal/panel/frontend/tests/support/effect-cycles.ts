/**
 * Effects that feed themselves.
 *
 * An `$effect` depends on every rune it reads while it runs, and reads inside a
 * function it calls count as its own. So an effect that starts a read, and a read
 * that sets `loading` to refuse a second read while one is in flight, form a
 * ring: finishing the read clears `loading`, clearing `loading` re-runs the
 * effect, and the effect starts another read.
 *
 * Nothing catches this. It compiles, `svelte-check` is silent, and the runtime's
 * own `effect_update_depth_exceeded` only fires for a loop that closes
 * synchronously - this one closes through the network, so it looks like an
 * ordinary page while it asks the server about 1600 times a second. It shipped
 * twice: the notification inbox asked from every page of the panel, and the Root
 * settings page asked whenever it was open.
 *
 * The rule is checked as source because the behaviour needs a browser, a network
 * and a scheduler, and the test runtime here has none of them. It reads what
 * Svelte reads - the component's own AST - rather than matching text, so a rename
 * or a reformat cannot slip past it.
 *
 * The fix is usually to wrap the call in `untrack`, so the effect depends on what
 * it is watching rather than on the machinery of the work it starts.
 *
 * What it cannot see, so that nobody reads a pass here as a proof: it follows
 * calls to functions written in the same component - by name, declared inside a
 * body, or run where they stand - and stops at a call through anything else. A
 * ring closed by `store.load()`, where the read lives in another module, is
 * beyond a rule that reads one file. `lib/request-rate` is the backstop for that
 * one: it watches what the panel actually asks for, whatever the cause.
 *
 * Some rings are meant to be there and settle by themselves: the effect that
 * warms the failure table reads `failurePage` and its answer fills it, and the
 * second run stops at the guard because the answer is now there. Whether a ring
 * settles is a termination proof, not something a reader of the syntax can
 * decide, so this asks the author instead. Write `effect settles:` and the reason
 * in a comment above the effect, and it is allowed. A ring that nobody can
 * explain is a ring nobody checked.
 */

import { parse } from 'svelte/compiler';

/** Written above an effect whose ring reaches a fixed point, with the reason why. */
export const SETTLES = 'effect settles:';

/** One state that an effect both depends on and writes through the work it starts. */
export interface EffectCycle {
  /** The `$state` or `$derived` in the ring. */
  state: string;
  /** What writes it: a function the effect calls, or the effect itself. */
  through: string;
}

interface Node {
  type: string;
  [key: string]: unknown;
}

const RUNES = new Set(['$state', '$derived']);
const EFFECTS = new Set(['$effect']);

export function findEffectCycles(source: string): EffectCycle[] {
  const script = instanceBody(source);
  if (script.length === 0) return [];

  const reactive = declaredRunes(script);
  if (reactive.size === 0) return [];
  const functions = declaredFunctions(script);
  const cycles: EffectCycle[] = [];

  for (const effect of effectBodies(script)) {
    if (settles(source, effect)) continue;
    const depends = new Set<string>();
    const writes = new Map<string, string>();
    collect(effect, 'the effect', { depends, functions, reactive, seen: new Set(), writes });
    for (const state of depends) {
      const through = writes.get(state);
      if (through !== undefined) cycles.push({ state, through });
    }
  }

  return cycles;
}

/** How far above an effect its own commentary is taken to reach. */
const PREAMBLE = 600;

/** Whether the author has said, above this effect, that its ring reaches a fixed point. */
function settles(source: string, effect: Node): boolean {
  const start = typeof effect.start === 'number' ? effect.start : 0;

  return source.slice(Math.max(0, start - PREAMBLE), start).includes(SETTLES);
}

interface Walk {
  depends: Set<string>;
  functions: Map<string, Node>;
  reactive: Set<string>;
  /** Functions already walked on this path, so mutual recursion terminates. */
  seen: Set<string>;
  /** State written anywhere in the work, against the name of what writes it. */
  writes: Map<string, string>;
}

/**
 * What a body depends on, and what it writes.
 *
 * Only what runs before the first `await` can be a dependency, because that is
 * all Svelte is still watching when the effect returns. The cut is the await's
 * own position rather than the statement holding it: `if (!loading) { loading =
 * true; items = await read(); }` is one statement, and cutting whole statements
 * threw the guard away with it and saw no ring at all.
 *
 * Writes count wherever they are - after the await, inside a `.then`, anywhere -
 * because a write is what re-runs the effect whenever it lands.
 */
function collect(body: Node, name: string, walk: Walk): void {
  const notReads = namesInWritePosition(body);
  const boundary = firstAwait(body);
  const immediate = new Set<Node>();
  /* Functions declared inside this body count too. An effect that declares its
     own `const run = async () => …` and calls it is the same ring written in one
     place instead of two. */
  const functions = new Map([...walk.functions, ...declaredFunctions(statementsOf(body))]);
  walkSync(body, (current) => {
    if (offsetOf(current) >= boundary) return;
    immediate.add(current);
    if (isRead(current, walk.reactive, notReads)) walk.depends.add(String(current.name));
    /* A function called where it is written runs now, so what it reads is the
       effect's. Nothing else would look into it: the walk steps over a function
       body, which is right for a callback somebody else will run and wrong for
       one being run on the spot. */
    const invoked = calledInPlace(current);
    if (invoked !== null) collect(invoked, name, { ...walk, functions });
    const called = calledLocal(current, functions);
    if (called === null || walk.seen.has(called)) return;
    walk.seen.add(called);
    const target = functions.get(called);
    if (target !== undefined) collect(target, called, { ...walk, functions });
  });
  walkAll(body, (current) => {
    /* Only a write that lands later can close the ring invisibly. One that lands
       now either settles - the guard that compares before it writes is how half
       the panel keeps a copy in step - or spins, and a ring that spins without
       ever awaiting is stopped by the runtime itself with
       `effect_update_depth_exceeded`. Nothing stops the other kind. */
    /* Where a write lands, not where it is written. `settings = await read()`
       begins before the await and completes after it, so the assignment's own
       position says immediate while the value arrives late. */
    if (immediate.has(current) && !awaitsWithin(current)) return;
    for (const written of assigned(current, walk.reactive)) {
      if (!walk.writes.has(written)) walk.writes.set(written, name);
    }
  });
}

/** Where this body's first await is, ignoring the ones belonging to functions inside it. */
function firstAwait(body: Node): number {
  let earliest = Number.POSITIVE_INFINITY;
  walkSync(body, (current) => {
    if (current.type !== 'AwaitExpression') return;
    earliest = Math.min(earliest, offsetOf(current));
  });

  return earliest;
}

function awaitsWithin(current: Node): boolean {
  return firstAwait(current) !== Number.POSITIVE_INFINITY;
}

function offsetOf(current: Node): number {
  return typeof current.start === 'number' ? current.start : 0;
}

/** The statements of a block body, or none for an expression body. */
function statementsOf(body: Node): Node[] {
  const block = node(body, 'body');

  return block !== null && block.type === 'BlockStatement' ? nodes(block, 'body') : [];
}

/**
 * The identifiers that are not reads: what is being assigned, and what is only a
 * name.
 *
 * `loading = false` mentions `loading` without reading it, and `settings.loading`
 * mentions it without meaning this one at all. Counting either would make an
 * effect depend on everything it sets.
 */
function namesInWritePosition(body: Node): Set<Node> {
  const ignored = new Set<Node>();
  const ignore = (candidate: Node | null): void => {
    if (candidate !== null && candidate.type === 'Identifier') ignored.add(candidate);
  };
  walkAll(body, (current) => {
    if (current.type === 'AssignmentExpression') ignore(node(current, 'left'));
    if (current.type === 'UpdateExpression') ignore(node(current, 'argument'));
    if (current.type === 'VariableDeclarator') ignore(node(current, 'id'));
    if (
      (current.type === 'MemberExpression' || current.type === 'Property') &&
      current.computed !== true
    ) {
      ignore(node(current, current.type === 'Property' ? 'key' : 'property'));
    }
    if (isFunction(current)) {
      for (const parameter of nodes(current, 'params')) ignore(parameter);
    }
  });

  return ignored;
}

/** A read of reactive state: an identifier that is not being written and is not a property name. */
function isRead(current: Node, reactive: Set<string>, notReads: Set<Node>): boolean {
  return (
    current.type === 'Identifier' && reactive.has(String(current.name)) && !notReads.has(current)
  );
}

/** The reactive state this node assigns to, if any. */
function assigned(current: Node, reactive: Set<string>): string[] {
  const target =
    current.type === 'AssignmentExpression'
      ? node(current, 'left')
      : current.type === 'UpdateExpression'
        ? node(current, 'argument')
        : null;
  if (target === null || target.type !== 'Identifier') return [];
  const name = String(target.name);

  return reactive.has(name) ? [name] : [];
}

/** The function this node calls where it stands, for `(async () => { … })()`. */
function calledInPlace(current: Node): Node | null {
  if (current.type !== 'CallExpression') return null;
  const callee = node(current, 'callee');

  return callee !== null && isFunction(callee) ? callee : null;
}

/** The name of a locally declared function this node calls, if it calls one. */
function calledLocal(current: Node, functions: Map<string, Node>): string | null {
  if (current.type !== 'CallExpression') return null;
  const callee = node(current, 'callee');
  if (callee === null || callee.type !== 'Identifier') return null;
  const name = String(callee.name);

  return functions.has(name) ? name : null;
}

/**
 * Walks the part that runs now: not into a nested function, and not into `untrack`.
 *
 * A function passed as an argument runs when its owner decides to run it, which
 * is not something this can know, so its reads are not the effect's. `untrack` is
 * the whole point of the rule - it is how a caller says the work it starts is not
 * what this effect is watching.
 */
function walkSync(root: Node, visit: (current: Node) => void): void {
  walk(root, visit, (current) => !isFunction(current) && !isUntrack(current));
}

function walkAll(root: Node, visit: (current: Node) => void): void {
  walk(root, visit, () => true);
}

function walk(root: Node, visit: (current: Node) => void, into: (current: Node) => boolean): void {
  visit(root);
  for (const child of children(root)) {
    if (into(child)) walk(child, visit, into);
  }
}

function isFunction(current: Node): boolean {
  return (
    current.type === 'ArrowFunctionExpression' ||
    current.type === 'FunctionExpression' ||
    current.type === 'FunctionDeclaration'
  );
}

function isUntrack(current: Node): boolean {
  if (current.type !== 'CallExpression') return false;
  const callee = node(current, 'callee');

  return callee !== null && callee.type === 'Identifier' && callee.name === 'untrack';
}

/** `let x = $state(...)` and `const x = $derived(...)`, by name. */
function declaredRunes(script: Node[]): Set<string> {
  const names = new Set<string>();
  for (const statement of script) {
    if (statement.type !== 'VariableDeclaration') continue;
    for (const declarator of nodes(statement, 'declarations')) {
      const id = node(declarator, 'id');
      const init = node(declarator, 'init');
      if (id === null || id.type !== 'Identifier' || init === null) continue;
      if (runeName(init) !== null) names.add(String(id.name));
    }
  }

  return names;
}

/** `$state`, `$state.raw`, `$derived`, `$derived.by`, or null for anything else. */
function runeName(init: Node): string | null {
  if (init.type !== 'CallExpression') return null;
  const callee = node(init, 'callee');
  if (callee === null) return null;
  if (callee.type === 'Identifier' && RUNES.has(String(callee.name))) return String(callee.name);
  if (callee.type === 'MemberExpression') {
    const object = node(callee, 'object');
    if (object !== null && object.type === 'Identifier' && RUNES.has(String(object.name))) {
      return String(object.name);
    }
  }

  return null;
}

function declaredFunctions(script: Node[]): Map<string, Node> {
  const functions = new Map<string, Node>();
  for (const statement of script) {
    if (statement.type === 'FunctionDeclaration') {
      const id = node(statement, 'id');
      if (id !== null && id.type === 'Identifier') functions.set(String(id.name), statement);
      continue;
    }
    if (statement.type !== 'VariableDeclaration') continue;
    for (const declarator of nodes(statement, 'declarations')) {
      const id = node(declarator, 'id');
      const init = node(declarator, 'init');
      if (id === null || id.type !== 'Identifier' || init === null || !isFunction(init)) continue;
      functions.set(String(id.name), init);
    }
  }

  return functions;
}

/** The callback of every `$effect(...)` and `$effect.pre(...)` in the script. */
function effectBodies(script: Node[]): Node[] {
  const bodies: Node[] = [];
  for (const statement of script) {
    walkAll(statement, (current) => {
      if (current.type !== 'CallExpression' || !isEffect(current)) return;
      const first = nodes(current, 'arguments')[0];
      if (first !== undefined && isFunction(first)) bodies.push(first);
    });
  }

  return bodies;
}

function isEffect(current: Node): boolean {
  const callee = node(current, 'callee');
  if (callee === null) return false;
  if (callee.type === 'Identifier') return EFFECTS.has(String(callee.name));
  if (callee.type !== 'MemberExpression') return false;
  const object = node(callee, 'object');

  return object !== null && object.type === 'Identifier' && EFFECTS.has(String(object.name));
}

function instanceBody(source: string): Node[] {
  const ast = parse(source, { modern: true }) as unknown as Node;
  const instance = node(ast, 'instance');
  if (instance === null) return [];
  const content = node(instance, 'content');

  return content === null ? [] : nodes(content, 'body');
}

function isNode(value: unknown): value is Node {
  return typeof value === 'object' && value !== null && typeof (value as Node).type === 'string';
}

function node(parent: Node, key: string): Node | null {
  const value = parent[key];

  return isNode(value) ? value : null;
}

function nodes(parent: Node, key: string): Node[] {
  const value = parent[key];

  return Array.isArray(value) ? value.filter(isNode) : [];
}

function children(parent: Node): Node[] {
  const found: Node[] = [];
  for (const [key, value] of Object.entries(parent)) {
    // The parser hangs the whole source off some nodes; walking back up is a loop.
    if (key === 'parent' || key === 'loc') continue;
    if (isNode(value)) found.push(value);
    else if (Array.isArray(value)) found.push(...value.filter(isNode));
  }

  return found;
}
