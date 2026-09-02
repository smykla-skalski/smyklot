<script lang="ts">
  import { readPanelBuild } from '#lib/base.js';
  import LegalPage from '#lib/components/LegalPage.svelte';

  const build = readPanelBuild(document);
</script>

<!--
  What using Smyklot means on both sides. Deliberately short and specific: the
  thing worth saying about a bot that can merge code is who is answerable for
  what it merges, and that is the section a reader came for.
-->

<LegalPage {build} title="Terms" updated="2 September 2026">
  <p>
    These terms cover the Smyklot service at smyklot.com and the GitHub App of the same name.
    Installing the app or signing in to the panel means you accept them.
  </p>

  <h2>What the service does</h2>
  <p>
    Smyklot acts on repositories you install it on: it approves and merges pull requests in response
    to slash commands from people your CODEOWNERS file names, applies the configuration you give it,
    and keeps a record of what it did. It only ever does what its configuration and that file
    permit.
  </p>

  <h2>Your side</h2>
  <ul>
    <li>
      You need the right to install the app on the accounts and repositories you install it on.
    </li>
    <li>
      You are responsible for what Smyklot does on your behalf. An approval it makes on your
      instruction is your approval, and a merge it performs is your merge — configure it
      accordingly, and review what you are asking it to do.
    </li>
    <li>
      Do not use it to work around a review requirement somebody else is relying on, and do not try
      to break, overload or reverse the service.
    </li>
  </ul>

  <h2>Our side</h2>
  <p>
    The service is provided as it is, without warranty. It depends on GitHub, and when GitHub is
    unavailable or refuses an operation, Smyklot cannot complete it. There is no uptime guarantee
    and no support commitment beyond best effort at
    <a href="mailto:support@smyklot.com">support@smyklot.com</a>.
  </p>
  <p>
    To the extent the law allows, Smyklot is not liable for indirect or consequential loss, or for
    anything arising from code merged on your instruction.
  </p>

  <h2>Ending it</h2>
  <p>
    Uninstall the GitHub App at any time and processing stops immediately. Access to the panel may
    be withdrawn from an account that abuses the service.
  </p>

  <h2>Changes</h2>
  <p>
    These terms may change. The date above says when they last did, and continuing to use the
    service after that is acceptance of the current version.
  </p>
</LegalPage>
