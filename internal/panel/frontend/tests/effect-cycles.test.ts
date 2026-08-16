import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

import { findEffectCycles, SETTLES } from './support/effect-cycles';
import { componentSources } from './support/markup';

/**
 * No effect may depend on state that the work it starts also writes.
 *
 * See `support/effect-cycles` for why this cannot be caught anywhere else. What
 * it costs when it is not caught: a page that looks ordinary while it asks the
 * server about 1600 times a second, and a failed read that never reaches the
 * screen because the next attempt clears it first.
 */

/* The shell holds effects of its own - the stream, preferences, and route
   resolution - so it is swept with the components rather than left exempt. */
const ROOT_LAYOUT = readFileSync(new URL('../src/routes/+layout.svelte', import.meta.url), 'utf8');

describe('effects that feed themselves [Unit]', () => {
  const sources = [...componentSources(), ['routes/+layout.svelte', ROOT_LAYOUT] as const];

  it.each(sources.map(([file]) => file))('%s starts no ring', (file) => {
    const source = sources.find(([name]) => name === file)?.[1] ?? '';
    expect(findEffectCycles(source).map(describeCycle)).toEqual([]);
  });

  /* The two shapes that shipped, kept as fixtures so the rule is proven to fail
     rather than assumed to. Written the way the components wrote them, down to
     the guard that reads the flag it sets, because that guard is the ring. */
  it('catches a read guarded by the flag the read sets', () => {
    expect(findEffectCycles(GUARD_FIXTURE)).toEqual([{ state: 'loading', through: 'load' }]);
  });

  it('catches a first-read test against the value the read fills', () => {
    expect(findEffectCycles(FIRST_READ_FIXTURE)).toEqual([{ state: 'settings', through: 'load' }]);
  });

  it('catches an effect that writes back what it read, after awaiting', () => {
    expect(findEffectCycles(DIRECT_FIXTURE)).toEqual([{ state: 'rows', through: 'the effect' }]);
  });

  /* The guard and the await in one statement. Cutting the body at the statement
     that awaits threw the guard away with it and reported nothing at all. */
  it('catches a guard that shares its statement with the await', () => {
    expect(findEffectCycles(ONE_STATEMENT_FIXTURE)).toEqual([
      { state: 'loading', through: 'load' },
    ]);
  });

  /* The guard read moved into a closure the loader declares - a refactor that
     changes nothing about the ring, and hid it from the first version of this. */
  it('catches a guard read through a closure inside the loader', () => {
    expect(findEffectCycles(NESTED_GUARD_FIXTURE)).toEqual([{ state: 'loading', through: 'load' }]);
  });

  /* Two more ways of spelling the same guard, both raised by a reviewer against
     an earlier version of this rule, which saw neither. */
  it('catches a guard called where it is written', () => {
    expect(findEffectCycles(IIFE_GUARD_FIXTURE)).toEqual([{ state: 'loading', through: 'load' }]);
  });

  it('catches a guard kept as a method on a local object', () => {
    expect(findEffectCycles(OBJECT_METHOD_FIXTURE)).toEqual([
      { state: 'loading', through: 'load' },
    ]);
  });

  it('catches a loader the effect runs where it stands', () => {
    expect(findEffectCycles(IIFE_FIXTURE)).toEqual([{ state: 'loading', through: 'the effect' }]);
  });

  it('catches a loader the effect declares for itself', () => {
    expect(findEffectCycles(INNER_FUNCTION_FIXTURE)).toEqual([
      { state: 'loading', through: 'run' },
    ]);
  });

  /* A read after the await is not a dependency: Svelte stopped watching when the
     effect returned. Flagging it would ask half the panel to untrack a ring that
     does not exist. */
  it('leaves a read that happens after the await alone', () => {
    expect(findEffectCycles(LATE_READ_FIXTURE)).toEqual([]);
  });

  it('lets untrack say the work is not what the effect watches', () => {
    expect(findEffectCycles(UNTRACKED_FIXTURE)).toEqual([]);
  });

  /* A ring that closes without awaiting either reaches a fixed point - which is
     how half the panel keeps a copy in step with what it mirrors - or spins, and
     a spinning one is stopped by the runtime with `effect_update_depth_exceeded`
     rather than by silently flooding the network. Not this rule's business. */
  it('leaves a ring that closes synchronously alone', () => {
    expect(findEffectCycles(SYNCHRONOUS_FIXTURE)).toEqual([]);
  });

  /* Reads inside a callback are not the effect's own: whoever holds the callback
     decides when it runs. Counting them would flag most of the panel. */
  it('leaves a read inside a callback out of the effect', () => {
    expect(findEffectCycles(CALLBACK_FIXTURE)).toEqual([]);
  });

  it('takes the author at their word when they say it settles', () => {
    expect(findEffectCycles(SETTLED_FIXTURE)).toEqual([]);
    expect(SETTLED_FIXTURE).toContain(SETTLES);
  });

  /* One effect's excuse must not cover the next one. A marker that reaches
     downwards is the one failure this rule cannot have: nothing would report the
     effect that got away, and the reason it got away would be invisible. */
  it('stops the excuse at the effect it was written for', () => {
    expect(findEffectCycles(MARKER_LEAK_FIXTURE)).toEqual([{ state: 'loading', through: 'load' }]);
  });
});

function describeCycle(cycle: { state: string; through: string }): string {
  return `${cycle.through} writes ${cycle.state}, which the effect reads`;
}

const GUARD_FIXTURE = `<script lang="ts">
  const { version }: { version: number } = $props();
  let loading = $state(false);
  let items = $state<string[]>([]);

  async function load(): Promise<void> {
    if (loading) return;
    loading = true;
    try {
      items = await fetch('/x').then((response) => response.json());
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    if (version >= 0) void load();
  });
</script>`;

const FIRST_READ_FIXTURE = `<script lang="ts">
  const { version }: { version: number } = $props();
  let settings = $state<object | null>(null);
  let loading = $state(true);

  async function load(): Promise<void> {
    loading = settings === null;
    settings = await fetch('/x').then((response) => response.json());
    loading = false;
  }

  $effect(() => {
    if (version >= 0) void load();
  });
</script>`;

const DIRECT_FIXTURE = `<script lang="ts">
  let rows = $state<string[]>([]);

  $effect(() => {
    const known = rows.length;
    void fetch('/x')
      .then((response) => response.json())
      .then((loaded) => {
        rows = [...loaded, known];
      });
  });
</script>`;

const ONE_STATEMENT_FIXTURE = `<script lang="ts">
  const { version }: { version: number } = $props();
  let loading = $state(false);
  let items = $state<string[]>([]);

  async function load(): Promise<void> {
    if (!loading) {
      loading = true;
      items = await fetch('/x').then((response) => response.json());
      loading = false;
    }
  }

  $effect(() => {
    if (version >= 0) void load();
  });
</script>`;

const NESTED_GUARD_FIXTURE = `<script lang="ts">
  const { version }: { version: number } = $props();
  let loading = $state(false);
  let items = $state<string[]>([]);

  async function load(): Promise<void> {
    const busy = (): boolean => loading;
    if (busy()) return;
    loading = true;
    try {
      items = await fetch('/x').then((response) => response.json());
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    if (version >= 0) void load();
  });
</script>`;

const INNER_FUNCTION_FIXTURE = `<script lang="ts">
  let loading = $state(false);
  let items = $state<string[]>([]);

  $effect(() => {
    const run = async (): Promise<void> => {
      if (loading) return;
      loading = true;
      items = await fetch('/x').then((response) => response.json());
      loading = false;
    };
    void run();
  });
</script>`;

const IIFE_GUARD_FIXTURE = `<script lang="ts">
  const { version }: { version: number } = $props();
  let loading = $state(false);
  let items = $state<string[]>([]);

  async function load(): Promise<void> {
    if ((() => loading)()) return;
    loading = true;
    try {
      items = await fetch('/x').then((response) => response.json());
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    if (version >= 0) void load();
  });
</script>`;

const OBJECT_METHOD_FIXTURE = `<script lang="ts">
  const { version }: { version: number } = $props();
  let loading = $state(false);
  let items = $state<string[]>([]);

  const guards = { active: (): boolean => loading };

  async function load(): Promise<void> {
    if (guards.active()) return;
    loading = true;
    try {
      items = await fetch('/x').then((response) => response.json());
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    if (version >= 0) void load();
  });
</script>`;

const IIFE_FIXTURE = `<script lang="ts">
  let loading = $state(false);
  let items = $state<string[]>([]);

  $effect(() => {
    void (async (): Promise<void> => {
      if (loading) return;
      loading = true;
      items = await fetch('/x').then((response) => response.json());
      loading = false;
    })();
  });
</script>`;

const LATE_READ_FIXTURE = `<script lang="ts">
  const { version }: { version: number } = $props();
  let page = $state<string[] | null>(null);

  async function load(): Promise<void> {
    const loaded = await fetch('/x').then((response) => response.json());
    if (page === null) page = loaded;
  }

  $effect(() => {
    if (version >= 0) void load();
  });
</script>`;

const SYNCHRONOUS_FIXTURE = `<script lang="ts">
  const { settings }: { settings: { revision: number } | null } = $props();
  let received = $state(-1);
  let amount = $state(0);

  $effect(() => {
    if (settings === null || settings.revision === received) return;
    received = settings.revision;
    amount = settings.revision * 2;
  });
</script>`;

const SETTLED_FIXTURE = `<script lang="ts">
  let page = $state<string[] | null>(null);

  /* effect settles: the read fills the page it is guarded by, so the run its own
     answer causes stops at the guard. */
  $effect(() => {
    if (page !== null) return;
    void fetch('/x')
      .then((response) => response.json())
      .then((loaded) => {
        page = loaded;
      });
  });
</script>`;

const MARKER_LEAK_FIXTURE = `<script lang="ts">
  let page = $state<string[] | null>(null);
  let loading = $state(false);
  let items = $state<string[]>([]);

  async function load(): Promise<void> {
    if (loading) return;
    loading = true;
    try {
      items = await fetch('/y').then((response) => response.json());
    } finally {
      loading = false;
    }
  }

  /* effect settles: the read fills the page it is guarded by. */
  $effect(() => {
    if (page !== null) return;
    void fetch('/x')
      .then((response) => response.json())
      .then((loaded) => {
        page = loaded;
      });
  });

  $effect(() => {
    void load();
  });
</script>`;

const UNTRACKED_FIXTURE = `<script lang="ts">
  import { untrack } from 'svelte';

  const { version }: { version: number } = $props();
  let loading = $state(false);
  let items = $state<string[]>([]);

  async function load(): Promise<void> {
    if (loading) return;
    loading = true;
    try {
      items = await fetch('/x').then((response) => response.json());
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    if (version >= 0) untrack(() => void load());
  });
</script>`;

const CALLBACK_FIXTURE = `<script lang="ts">
  let rows = $state<string[]>([]);

  $effect(() => {
    const listener = (): void => {
      rows = [...rows, 'more'];
    };
    window.addEventListener('resize', listener);
    return () => window.removeEventListener('resize', listener);
  });
</script>`;
