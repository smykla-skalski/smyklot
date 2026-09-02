<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import LegalPage from '#lib/components/LegalPage.svelte';

  const BUILD = { version: '1.50.2', serviceHost: 'smyklot.com' };

  const { Story } = defineMeta({
    title: 'Views/LegalPage',
    component: LegalPage,
    args: { build: BUILD, title: 'Privacy', updated: '2 September 2026' },
  });
</script>

<!--
  The shell the privacy notice and the terms stand on - the same night page as the
  sign-in card, because these are read by somebody with no session who followed a link
  from it.

  The prose is the caller's, so this shows the shapes it has to style rather than either
  document's actual words: a lead paragraph, a section heading, a list, and a link. Both
  real pages are made only of these.
-->
<Story name="Privacy">
  {#snippet template(args)}
    <LegalPage build={args.build} title={args.title} updated={args.updated}>
      <p>
        Smyklot is a GitHub App that approves and merges pull requests on repositories you have
        installed it on.
      </p>
      <h2>What is stored</h2>
      <p>Signing in with GitHub stores, for as long as your account exists:</p>
      <ul>
        <li>Your GitHub account id, login, display name and avatar URL.</li>
        <li>A session record per browser, holding a hash of the token rather than the token.</li>
        <li>An audit trail of what was done through the panel, and when.</li>
      </ul>
      <h2>Your choices</h2>
      <p>
        Uninstalling the GitHub App ends Smyklot's access immediately. To ask what is held, write to <a
          href="mailto:support@smyklot.com">support@smyklot.com</a
        >.
      </p>
    </LegalPage>
  {/snippet}
</Story>

<!-- The same shell, the other document. One component, two sets of words. -->
<Story name="Terms" args={{ title: 'Terms' }}>
  {#snippet template(args)}
    <LegalPage build={args.build} title={args.title} updated={args.updated}>
      <p>
        These terms cover the Smyklot service and the GitHub App of the same name. Installing the
        app or signing in to the panel means you accept them.
      </p>
      <h2>Your side</h2>
      <ul>
        <li>You need the right to install the app where you install it.</li>
        <li>An approval it makes on your instruction is your approval.</li>
      </ul>
    </LegalPage>
  {/snippet}
</Story>
