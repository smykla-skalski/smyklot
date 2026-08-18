<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import InvitationPage from '#lib/components/InvitationPage.svelte';
  import Seeded from '../support/Seeded.svelte';
  import { stubApi } from '../support/api.js';
  import { ACCOUNT, NOW } from '../support/fixtures.js';
  import type { PanelInvitation } from '#lib/types.js';

  const at = (offsetMs: number) => new Date(NOW + offsetMs).toISOString();
  const TOKEN = 'inv-8f2c1d9e';
  const KEY = ['invitation', TOKEN] as const;
  const BUILD = { version: '1.37.0', serviceHost: 'smyklot.com' };

  const invitation = (over: Partial<PanelInvitation> = {}): PanelInvitation => ({
    id: 'inv-1',
    account: { ...ACCOUNT, id: '1001', login: 'ada', display_name: 'Ada Lovelace' },
    target_name: 'Smykla Skalski',
    target_login: 'smykla-skalski',
    target_kind: 'Organization',
    role: 'editor',
    status: 'pending',
    expires_at: at(7 * 24 * 60 * 60_000),
    created_by: { ...ACCOUNT, id: '1002', login: 'bart', display_name: 'Bart Smykla' },
    created_at: at(-60 * 60_000),
    ...over,
  });

  const base = { api: stubApi(), base: '', token: TOKEN, build: BUILD };

  const { Story } = defineMeta({
    title: 'Views/InvitationPage',
    component: InvitationPage,
    args: base,
  });
</script>

<!--
  The page a stranger opens from a link in their email. Every state it can be in - a
  token that is live, one already answered, one that has run out, and one that names
  nothing at all.
-->
<Story name="Pending">
  {#snippet template(args)}
    <Seeded seed={[[KEY, invitation()]]}><InvitationPage {...args} /></Seeded>
  {/snippet}
</Story>

<Story name="Accepted">
  {#snippet template(args)}
    <Seeded seed={[[KEY, invitation({ status: 'accepted', responded_at: at(-30 * 60_000) })]]}>
      <InvitationPage {...args} />
    </Seeded>
  {/snippet}
</Story>

<Story name="Declined">
  {#snippet template(args)}
    <Seeded seed={[[KEY, invitation({ status: 'declined', responded_at: at(-30 * 60_000) })]]}>
      <InvitationPage {...args} />
    </Seeded>
  {/snippet}
</Story>

<Story name="Expired">
  {#snippet template(args)}
    <Seeded seed={[[KEY, invitation({ status: 'expired', expires_at: at(-24 * 60 * 60_000) })]]}>
      <InvitationPage {...args} />
    </Seeded>
  {/snippet}
</Story>

<!-- A Root invitation has no workspace: the sentence ends where it stands. -->
<Story name="Root role">
  {#snippet template(args)}
    <Seeded
      seed={[
        [
          KEY,
          invitation({
            system_role: 'root',
            role: undefined,
            target_name: undefined,
            target_login: undefined,
            target_kind: undefined,
          }),
        ],
      ]}
    >
      <InvitationPage {...args} />
    </Seeded>
  {/snippet}
</Story>

<!--
  A token that names nothing is answered the way every dead address is - a 404 in the
  same words, without naming invitations. Telling a reader which feature the address
  would have belonged to only describes something they cannot reach.
-->
<Story name="Token names nothing">
  {#snippet template(args)}
    <Seeded>
      <InvitationPage
        {...args}
        api={stubApi({
          fetchInvitation: async () => {
            throw Object.assign(new Error('not found'), { status: 404 });
          },
        })}
      />
    </Seeded>
  {/snippet}
</Story>

<Story name="Loading">
  {#snippet template(args)}
    <Seeded>
      <InvitationPage {...args} api={stubApi({ fetchInvitation: () => new Promise(() => {}) })} />
    </Seeded>
  {/snippet}
</Story>
