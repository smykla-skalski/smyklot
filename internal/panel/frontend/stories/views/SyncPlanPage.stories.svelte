<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import SyncPlanPage from '#lib/components/SyncPlanPage.svelte';
  import type { SyncAction, SyncPlan } from '#lib/types.js';

  const NOW = Date.UTC(2026, 7, 18, 12, 0, 0);
  const minutes = (count: number): string => new Date(NOW - count * 60_000).toISOString();

  const DEPENDENCIES = '#0e8a16, "Dependency updates, mostly Renovate\'s"';
  const RENOVATE_BEFORE = [
    '"extends": ["config:recommended"],',
    '"schedule": ["* 4 * * 0"],',
    '"packageRules": [',
  ].join('\n');
  const RENOVATE_AFTER = [
    '"extends": ["config:recommended"],',
    '"schedule": ["* 4 * * 1-5"],',
    '"timezone": "Europe/Warsaw",',
    '"packageRules": [',
  ].join('\n');

  /* The approved mock's fourteen rows, verbatim: af +3 ~2 −1, afi +3 ~2,
     harness +2 ~1. */
  const ACTIONS: SyncAction[] = [
    {
      repository: 'platform-infra',
      kind: 'labels',
      operation: 'create',
      subject: 'dependencies',
      after: DEPENDENCIES,
      state: 'pending',
    },
    {
      repository: 'platform-infra',
      kind: 'labels',
      operation: 'create',
      subject: 'good first issue',
      state: 'pending',
    },
    {
      repository: 'platform-infra',
      kind: 'settings',
      operation: 'update',
      subject: 'squash merging',
      before: 'off',
      after: 'on',
      state: 'pending',
    },
    {
      repository: 'platform-infra',
      kind: 'settings',
      operation: 'update',
      subject: 'wiki',
      before: 'on',
      after: 'off',
      state: 'pending',
    },
    {
      repository: 'platform-infra',
      kind: 'files',
      operation: 'create',
      subject: 'renovate.json',
      before: RENOVATE_BEFORE,
      after: RENOVATE_AFTER,
      state: 'pending',
    },
    {
      repository: 'platform-infra',
      kind: 'files',
      operation: 'delete',
      subject: '.github/stale.yml',
      state: 'pending',
    },
    {
      repository: 'api-gateway',
      kind: 'settings',
      operation: 'create',
      subject: 'delete branch on merge',
      after: 'on',
      state: 'pending',
    },
    {
      repository: 'api-gateway',
      kind: 'settings',
      operation: 'create',
      subject: 'auto-merge',
      after: 'on',
      state: 'pending',
    },
    {
      repository: 'api-gateway',
      kind: 'settings',
      operation: 'update',
      subject: 'squash merging',
      before: 'off',
      after: 'on',
      state: 'pending',
    },
    {
      repository: 'api-gateway',
      kind: 'settings',
      operation: 'update',
      subject: 'wiki',
      before: 'on',
      after: 'off',
      state: 'pending',
    },
    {
      repository: 'api-gateway',
      kind: 'rulesets',
      operation: 'create',
      subject: 'main-protection',
      after: '6 rules, active',
      state: 'pending',
    },
    {
      repository: 'auth-service',
      kind: 'labels',
      operation: 'create',
      subject: 'dependencies',
      after: DEPENDENCIES,
      state: 'pending',
    },
    {
      repository: 'auth-service',
      kind: 'labels',
      operation: 'create',
      subject: 'good first issue',
      state: 'pending',
    },
    {
      repository: 'auth-service',
      kind: 'settings',
      operation: 'update',
      subject: 'projects',
      before: 'on',
      after: 'off',
      state: 'pending',
    },
  ];

  const PLAN: SyncPlan = {
    id: 'plan-1',
    trigger: 'sweep',
    state: 'computed',
    execution_stage: 'Waiting for approval',
    digest: 'digest',
    counts: { create: 8, update: 5, delete: 1 },
    actions: ACTIONS,
    computed_at: minutes(12),
    expires_at: new Date(NOW + 6 * 3_600_000 + 5 * 60_000).toISOString(),
  };

  const { Story } = defineMeta({
    title: 'Views/SyncPlanPage',
    component: SyncPlanPage,
    args: {
      plan: PLAN,
      nowMs: NOW,
      readOnly: false,
      approving: false,
      discarding: false,
      onApprove: fn(),
      onDiscard: fn(),
    },
  });
</script>

<!--
  The plan: the verdict is the hero, the kind filter narrows the rows
  instantly, each repository's group carries its own operation counts, and
  the apply bar holds the breakdown, the promise and the scope. The
  lifecycle card below explains the six states once, at this plan's scale.
-->
<Story name="Waiting for you" />

<!-- No plan at all: nothing is waiting, which is also what the moment after
     a save looks like - the sentence claims no more than that. -->
<Story name="Nothing waiting" args={{ plan: null }} />

<!-- A failed apply re-renders the same rows: the error inline on the row
     that failed, "not tried" on everything behind it. -->
<Story
  name="Failed"
  args={{
    plan: {
      ...PLAN,
      state: 'failed',
      finished_at: minutes(3),
      actions: [
        {
          repository: 'platform-infra',
          kind: 'settings',
          operation: 'update',
          subject: 'squash merging',
          before: 'off',
          after: 'on',
          state: 'failed',
          error: "GitHub answered 403: the App's Administration permission was revoked",
        },
        {
          repository: 'platform-infra',
          kind: 'settings',
          operation: 'update',
          subject: 'wiki',
          before: 'on',
          after: 'off',
          state: 'skipped',
          blocker: 'squash merging',
        },
        {
          repository: 'platform-infra',
          kind: 'labels',
          operation: 'create',
          subject: 'dependencies',
          after: DEPENDENCIES,
          state: 'applied',
        },
      ],
    },
  }}
/>

<!-- A reader without write: the rows say everything, the bar stands down. -->
<Story name="Read only" args={{ readOnly: true }} />
