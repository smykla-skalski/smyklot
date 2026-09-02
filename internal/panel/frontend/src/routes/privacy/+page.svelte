<script lang="ts">
  import { readPanelBuild } from '#lib/base.js';
  import LegalPage from '#lib/components/LegalPage.svelte';

  const build = readPanelBuild(document);
</script>

<!--
  What Smyklot holds about a person and why. Written from what the code actually
  stores rather than from a template: the account row, the session row and the
  audit trail are the whole of it, and each is named here with the reason it
  exists. If the panel starts keeping something else, this is where it is said.
-->

<LegalPage {build} title="Privacy" updated="2 September 2026">
  <p>
    Smyklot is a GitHub App that approves and merges pull requests on repositories you have
    installed it on. This notice covers the Smyklot service at smyklot.com and the panel you sign in
    to.
  </p>

  <h2>What is stored</h2>
  <p>Signing in with GitHub stores, for as long as your account exists:</p>
  <ul>
    <li>
      Your GitHub account id, login, display name and avatar URL, so the panel can show who did
      what.
    </li>
    <li>
      A session record for each browser you sign in from. It holds a hash of the session token, not
      the token itself, and it expires.
    </li>
    <li>
      An audit trail of actions taken through the panel — who changed a setting, approved a plan or
      answered a queued request, and when.
    </li>
    <li>Your timezone, if you set one, so times are shown in it rather than in the server's.</li>
  </ul>
  <p>
    Repository and pull request data is read from GitHub when it is needed and is not copied into a
    database of our own, apart from what an audit entry names in order to be readable later.
  </p>

  <h2>What is not stored</h2>
  <p>
    The OAuth token from signing in is used for one request — asking GitHub which account you are —
    and then discarded. Sign-in uses a scopeless OAuth App, so GitHub is asked for your public
    profile and nothing else. Smyklot never receives your GitHub password. Source code is not
    retained after the operation that read it.
  </p>

  <h2>Who it is shared with</h2>
  <p>
    Nobody. Data is not sold, rented or given to advertisers, and there is no third-party analytics
    or tracking in the panel. It is processed by the hosting and database providers that run the
    service, and disclosed further only where the law requires it.
  </p>

  <h2>Where it is held</h2>
  <p>
    On Fly.io infrastructure in the EU, in a PostgreSQL database that is encrypted at rest. Traffic
    to the panel is served over HTTPS only.
  </p>

  <h2>How long it is kept</h2>
  <p>
    Account and audit records are kept while the installation exists, because an audit trail that is
    deleted piecemeal cannot be relied on. Sessions expire on their own. Removing the Smyklot GitHub
    App from an account stops all processing for it.
  </p>

  <h2>Your choices</h2>
  <p>
    You can sign out of any session from the panel, and you can uninstall the GitHub App at any time
    from your GitHub settings, which ends Smyklot's access immediately. To ask what is held about
    you, or to have it deleted, write to
    <a href="mailto:support@smyklot.com">support@smyklot.com</a>.
  </p>

  <h2>Changes</h2>
  <p>
    If this notice changes in a way that affects what is collected or who it is shared with, the
    date above changes with it.
  </p>
</LegalPage>
