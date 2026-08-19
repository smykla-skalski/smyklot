<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import SyncRulesetsForm from '#lib/components/SyncRulesetsForm.svelte';
  import type { SyncState } from '#lib/components/StateMark.svelte';

  const STORED = {
    rulesets: [
      {
        name: 'main-branch-protection',
        target: 'branch',
        enforcement: 'active',
        conditions: { include: ['~DEFAULT_BRANCH'], exclude: [] },
        rules: {
          deletion: true,
          non_fast_forward: true,
          required_linear_history: true,
          required_signatures: true,
          pull_request: { required_approving_review_count: 1, allowed_merge_methods: ['squash'] },
          required_status_checks: { required_status_checks: [{ context: 'test' }] },
        },
        bypass_actors: [{ actor_id: 0, actor_type: 'OrganizationAdmin', bypass_mode: 'always' }],
      },
      {
        name: 'release-tags',
        target: 'tag',
        enforcement: 'evaluate',
        conditions: { include: ['refs/tags/v*'], exclude: [] },
        rules: { creation: true, deletion: true },
      },
    ],
    allow_removal: false,
    excludes: ['hand-made-*'],
  };

  const MARKS: Record<string, { state: SyncState; label?: string }> = {
    'main-branch-protection': { state: 'change', label: '1 repository differs' },
    'release-tags': { state: 'settled' },
  };

  const { Story } = defineMeta({
    title: 'Views/SyncRulesetsForm',
    component: SyncRulesetsForm,
    argTypes: {
      enabled: { control: 'boolean' },
      unreadable: { control: 'boolean' },
      readOnly: { control: 'boolean' },
      saving: { control: 'boolean' },
    },
    args: {
      stored: STORED,
      enabled: true,
      unreadable: false,
      readOnly: false,
      saving: false,
      rulesetHref: (name: string) => `#ruleset-${name}`,
      markOf: (name: string) => MARKS[name],
      onSave: fn(),
    },
  });
</script>

<!--
  Two levels and no deeper: this page answers "which rulesets, and is each one
  holding", and a row opens the ruleset's own page for everything else. The
  whole configuration used to be one page - nine rules, their parameters, their
  bypass actors and their ref patterns, per ruleset, unfolded.
-->
<Story name="Two rulesets" />

<!-- No plan has been worked out, so no row claims to be in step: a ruleset with
     no action in a plan is settled, but with no plan it has not been looked at. -->
<Story name="Before a plan" args={{ markOf: () => undefined }} />

<!-- Nothing configured yet: the page has to offer a way in rather than a void. -->
<Story name="Empty" args={{ stored: { rulesets: [] } }} />

<Story name="Switched off" args={{ enabled: false }} />

<Story name="Saving" args={{ saving: true }} />

<Story name="Unreadable" args={{ unreadable: true }} />

<Story name="Read only" args={{ readOnly: true }} />

<!--
  The save was refused, and the message belongs beside this form rather than at the top
  of the page. The four forms save independently, so a shared message is one form's
  failure wiped by the next form's click.
-->
<Story
  name="With a problem"
  args={{ problem: 'GitHub refused the write: 422 invalid bypass actor' }}
/>

<!--
  The installation has not granted what ruleset sync needs. Rulesets and settings ask
  for the same permission, so granting it answers both at once.
-->
<Story
  name="Unavailable"
  args={{
    unavailable: 'Smyklot has not been granted administration access, which rulesets sync needs',
  }}
/>
