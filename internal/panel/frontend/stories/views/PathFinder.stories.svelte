<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import PathFinder, { type KnownPath } from '#lib/components/PathFinder.svelte';

  const { Story } = defineMeta({
    title: 'Views/PathFinder',
    component: PathFinder,
  });

  /** What an installation's index looks like: one row per distinct path. */
  const KNOWN: KnownPath[] = [
    { path: '.github/CODEOWNERS', repositories: 25 },
    { path: '.github/dependabot.yml', repositories: 19 },
    { path: '.github/pull_request_template.md', repositories: 22 },
    { path: '.github/workflows/test.yaml', repositories: 24 },
    { path: '.github/workflows/lint.yaml', repositories: 17 },
    { path: '.github/workflows/release.yaml', repositories: 12 },
    { path: '.github/workflows/codeql.yaml', repositories: 8 },
    { path: '.github/ISSUE_TEMPLATE/bug.yaml', repositories: 6 },
    { path: '.editorconfig', repositories: 21 },
    { path: '.gitignore', repositories: 25 },
    { path: 'renovate.json', repositories: 23 },
    { path: 'CONTRIBUTING.md', repositories: 11 },
    { path: 'LICENSE', repositories: 25 },
    { path: 'README.md', repositories: 25 },
    { path: 'SECURITY.md', repositories: 9 },
    { path: 'mise.toml', repositories: 14 },
    { path: 'docs/releasing.md', repositories: 4 },
    { path: 'scripts/bootstrap.sh', repositories: 7 },
  ];
</script>

<!--
  Type into the field. `wf` reaches the workflows, `cow` reaches CODEOWNERS
  from its initials, and `rel` puts `release.yaml` above `docs/releasing.md`
  because a match inside the file name is a different kind of answer until the
  query carries a `/`. The characters that matched are painted from the same
  walk that ranked the row.

  The count on the right is why this is one row and not twenty-five: the path
  is the thing being configured, and how many repositories already hold it is
  the fact worth carrying beside it.
-->
<Story name="Finding a path">
  {#snippet template()}
    <div class="frame">
      <PathFinder paths={KNOWN} repositories={25} label="Path in each repository" />
    </div>
  {/snippet}
</Story>

<!-- A path no repository has yet is still a path, so the finder offers it. -->
<Story name="A path that does not exist yet">
  {#snippet template()}
    <div class="frame">
      <PathFinder
        paths={KNOWN}
        repositories={25}
        value=".github/workflows/sync.yaml"
        label="Path in each repository"
      />
    </div>
  {/snippet}
</Story>

<!--
  The one way this list can be short. Nothing drops a path on purpose - the cap
  that used to is gone, and a tree GitHub will not list whole is divided into
  subtrees until it answers. What survives even that is said here, because a
  short list that looks complete is what makes somebody believe a file they can
  see in their own repository is not there.
-->
<Story name="A list GitHub would not finish">
  {#snippet template()}
    <div class="frame">
      <PathFinder paths={KNOWN} repositories={25} partial label="Path in each repository" />
    </div>
  {/snippet}
</Story>

<style>
  /* Room for the list to open into, which a story otherwise has to be scrolled
     to see. */
  .frame {
    min-height: 26rem;
  }
</style>
