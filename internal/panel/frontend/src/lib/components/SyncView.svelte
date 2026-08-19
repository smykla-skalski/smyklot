<script lang="ts">
  /**
   * Org-wide label sync: what an installation expects its repositories to carry,
   * and what the service would change to make that true.
   *
   * Two halves, in the order the questions arrive. What is configured, which is
   * the thing somebody edits. Then what that would do, which is the thing
   * somebody approves - and those are deliberately not the same act. A sync that
   * applied on save would give nobody the chance to read the deletions first.
   */
  import { untrack } from 'svelte';
  import { useInterval } from 'runed';

  import { storedList } from '#lib/form-lists.js';
  import { formatRelative, formatUntil } from '#lib/format.js';

  import { SYNC_SECTIONS, type SyncPage, type SyncSection } from '#lib/routes.js';
  import type {
    Page,
    RepositorySummary,
    SyncAction,
    SyncConfig,
    SyncConfigInput,
    SyncFile,
    SyncLabel,
    SyncOverrideInput,
    SyncOverrideRow,
    SyncPlan,
    SyncRuleset,
  } from '#lib/types.js';

  import FormError from './FormError.svelte';
  import Button from './Button.svelte';
  import Icon from './Icon.svelte';
  import PageHeader from './PageHeader.svelte';
  import ApplyBar from './ApplyBar.svelte';
  import PlanAction, { type PlanOp } from './PlanAction.svelte';
  import PlanGroup from './PlanGroup.svelte';
  import type { KnownPath } from './PathFinder.svelte';
  import SectionTabs from './SectionTabs.svelte';
  import StateMark, { type SyncState } from './StateMark.svelte';
  import SyncBoard, { type BoardRepository } from './SyncBoard.svelte';
  import SyncFileDetail from './SyncFileDetail.svelte';
  import SyncFilesForm from './SyncFilesForm.svelte';
  import SyncKindCard from './SyncKindCard.svelte';
  import SyncLabelsForm from './SyncLabelsForm.svelte';
  import SyncRulesetDetail from './SyncRulesetDetail.svelte';
  import SyncRulesetsForm from './SyncRulesetsForm.svelte';
  import SyncSettingsForm, { SETTING_KEYS } from './SyncSettingsForm.svelte';

  const {
    targetId,
    readOnly,
    account = '',
    section = 'overview',
    item,
    sectionHref,
    fetchRepositories,
    repositoryHref,
    fetchConfig,
    saveConfig,
    fetchOverrides,
    saveOverride,
    fetchPaths,
    fetchPlan,
    approvePlan,
  }: {
    targetId: string;
    readOnly: boolean;
    /** The installation, said once in the overview's eyebrow. */
    account?: string;
    /** Which section the address names; the overview is the bare address. */
    section?: SyncSection;
    /**
     * The one thing inside that section the address names - a ruleset by name,
     * a file by path. Absent on the section's own list.
     */
    item?: string;
    /** Where each section lives, so the strip is real links rather than state. */
    sectionHref: (page: SyncPage) => string;
    /**
     * The population the board draws. One page: the board is a shape to read at
     * a glance, and what it cannot show it says rather than pages through.
     */
    fetchRepositories?: (targetId: string) => Promise<Page<RepositorySummary>>;
    /** Where one repository's own page is, for the tiles and the list. */
    repositoryHref?: (name: string) => string;
    fetchConfig: (targetId: string, kind: string) => Promise<SyncConfig>;
    saveConfig: (targetId: string, kind: string, input: SyncConfigInput) => Promise<SyncConfig>;
    /**
     * Every repository's answer about one kind, in one request. The files pages
     * ask "who adjusts this file", which is a question about the installation.
     */
    fetchOverrides?: (targetId: string, kind: string) => Promise<{ overrides: SyncOverrideRow[] }>;
    /** Writes one repository's adjustment, from the page about the file. */
    saveOverride?: (repositoryId: string, input: SyncOverrideInput) => Promise<unknown>;
    /** Every path this installation's repositories are known to hold. */
    fetchPaths?: (
      targetId: string,
    ) => Promise<{ paths: KnownPath[]; repositories: number; partial?: boolean }>;
    fetchPlan: (targetId: string) => Promise<{ plan: SyncPlan | null }>;
    approvePlan: (targetId: string, planId: string, digest: string) => Promise<{ plan: SyncPlan }>;
  } = $props();

  // The kinds this view has a form for, named rather than taken as a parameter
  // nothing varies.
  // `as const`, because these name sections as well as kinds: without the
  // literal type a card's `href` is a string, and every route the section
  // vocabulary keys stops being checked.
  const LABELS = 'labels' as const;
  const SETTINGS = 'settings' as const;
  const RULESETS = 'rulesets' as const;
  const FILES = 'files' as const;

  /**
   * The kinds whose whole configuration is one document. Labels are not one of
   * them: their form was built out of typed fields before there was a second
   * kind, and everything since travels as a document.
   */
  type DocumentKind = typeof SETTINGS | typeof RULESETS | typeof FILES;

  let config = $state<SyncConfig | null>(null);
  let plan = $state<SyncPlan | null>(null);
  /** The board's population, and what the page could not fit on it. */
  let fleet = $state<Page<RepositorySummary> | null>(null);
  /** Every repository's answer about files, which the files pages read. */
  let adjustments = $state<SyncOverrideRow[]>([]);
  /** Every path this installation's repositories hold, for the finder. */
  let known = $state<{ paths: KnownPath[]; repositories: number; partial?: boolean }>({
    paths: [],
    repositories: 0,
  });
  let saving = $state(false);
  let approving = $state(false);

  /* Every relative time on this page is read against one clock, ticking slowly:
     "worked out 12 minutes ago" that never becomes thirteen is a page quietly
     describing the moment it loaded. */
  let now = $state(Date.now());
  useInterval(30_000, { callback: () => (now = Date.now()) });

  /* Kept per kind rather than as a field each, because every kind after labels
     has the same three: what is saved, whether a save is in flight, and what
     went wrong. Three fields per kind is how the third one comes to reuse the
     second one's by accident. */
  let documents = $state<Record<DocumentKind, SyncConfig | null>>({
    settings: null,
    rulesets: null,
    files: null,
  });
  let savingDocument = $state<Record<DocumentKind, boolean>>({
    settings: false,
    rulesets: false,
    files: false,
  });

  /* One failure per thing that can fail, because the forms are saved
     independently and none disables the others. A single field let a settings
     save clear a labels failure the moment it started - the label switch had
     already sprung back and nothing on the page said why. */
  let error = $state<string | null>(null);
  let documentError = $state<Record<DocumentKind, string | null>>({
    settings: null,
    rulesets: null,
    files: null,
  });

  /* Every document, because each is only meaningful beside the plan: a plan
     says what would change, and what it would change to is what the
     configurations list. untrack keeps the writes below from feeding back into
     the read that caused them. */
  $effect(() => {
    const id = targetId;
    untrack(() => void load(id));
  });

  async function load(id: string): Promise<void> {
    error = null;
    documentError = { settings: null, rulesets: null, files: null };
    try {
      const [loadedConfig, loadedSettings, loadedRulesets, loadedFiles, loadedPlan] =
        await Promise.all([
          fetchConfig(id, LABELS),
          fetchConfig(id, SETTINGS),
          fetchConfig(id, RULESETS),
          fetchConfig(id, FILES),
          fetchPlan(id),
        ]);
      config = loadedConfig;
      documents = { settings: loadedSettings, rulesets: loadedRulesets, files: loadedFiles };
      plan = loadedPlan.plan;
    } catch (cause) {
      error = messageOf(cause);
    }

    /* Separately, and never fatally: the board is the overview's instrument and
       the four forms are the page's job. A repository list that fails to arrive
       costs the board, and the sections still configure what they configure.
       The same for the two the files pages read: a finder with no index is a
       plain field, and a file page with no adjustments says nobody adjusts it,
       which is what an installation where nobody does looks like anyway. */
    /* Together, because none of the three needs another's answer and the page
       cannot draw until the slowest has arrived either way - in turn it waited
       for the sum of them. Each keeps its own landing, which is what lets the
       page still draw the other two when one is the one that failed. */
    await Promise.all([
      fetchRepositories?.(id).then(
        (answer) => (fleet = answer),
        () => (fleet = null),
      ),
      fetchOverrides?.(id, FILES).then(
        (answer) => (adjustments = answer.overrides),
        () => (adjustments = []),
      ),
      fetchPaths?.(id).then(
        (answer) => (known = answer),
        () => (known = { paths: [], repositories: 0 }),
      ),
    ]);
  }

  /**
   * Writes one repository's adjustment, from the page about the file.
   *
   * The whole list is re-read afterwards rather than patched in place: the
   * revision moved, and a page holding the old one answers every later save
   * with a conflict about the reader's own change.
   */
  /**
   * Writes one repository's adjustment, and says whether it landed.
   *
   * The answer is what lets an editor stay open over a rejection. It used to
   * return nothing, so the file detail closed its editor before the write and
   * a 409 threw away what somebody had typed - the page said why and the words
   * were already gone.
   */
  async function onSaveAdjustment(
    repositoryId: string,
    document: Record<string, unknown>,
  ): Promise<boolean> {
    const row = adjustments.find((candidate) => candidate.repository_id === repositoryId);
    if (row === undefined || saveOverride === undefined || fetchOverrides === undefined) {
      return false;
    }

    savingDocument = { ...savingDocument, files: true };
    documentError = { ...documentError, files: null };
    try {
      // The switch travels back as it was: this page edits what a repository
      // adjusts, and sending `null` for it would quietly re-inherit a kind
      // somebody had switched off there.
      await saveOverride(repositoryId, {
        enabled: row.enabled,
        document,
        expected_revision: row.revision,
      });
      // Together: the plan is computed from what the service holds rather than
      // from the list read back here, so neither answer waits on the other.
      const [rows, computed] = await Promise.all([
        fetchOverrides(targetId, FILES),
        fetchPlan(targetId),
      ]);
      adjustments = rows.overrides;
      plan = computed.plan;

      return true;
    } catch (cause) {
      documentError = { ...documentError, files: messageOf(cause) };

      return false;
    } finally {
      savingDocument = { ...savingDocument, files: false };
    }
  }

  /**
   * Saves the labels an installation expects, and whether it syncs them at all.
   *
   * One call rather than two, because they are one document: the switch and the
   * rows share a revision, and saving either has to send the other as it stands
   * or the server's copy of it is replaced by whatever this page last read.
   */
  async function onSaveLabels(
    enabled: boolean,
    wanted?: SyncLabel[],
    allowRemoval?: boolean,
    excludes?: string[],
  ): Promise<void> {
    const current = config;
    if (current === null) return;

    saving = true;
    error = null;
    try {
      config = await saveConfig(targetId, LABELS, {
        enabled,
        labels: wanted ?? current.labels,
        allow_removal: allowRemoval ?? current.allow_removal,
        excludes: excludes ?? current.excludes,
        expected_revision: current.revision,
      });
      // Saving invalidates any plan computed from the old configuration, so the
      // one on screen is re-read rather than left describing something that is
      // no longer true.
      plan = (await fetchPlan(targetId)).plan;
    } catch (cause) {
      error = messageOf(cause);
    } finally {
      saving = false;
    }
  }

  /**
   * A document kind is saved whole, unlike the labels switch beside it: a
   * repository's settings are one request that succeeds or fails together, and
   * a ruleset is written by replacement, so a control that saved on every click
   * would send a dozen half-formed policies and compute a plan for each.
   */
  /** Writes one kind's document, and says whether it landed. See onSaveAdjustment. */
  async function onSaveDocument(
    kind: DocumentKind,
    wanted: boolean,
    document: Record<string, unknown>,
  ): Promise<boolean> {
    const current = documents[kind];
    if (current === null) return false;

    savingDocument = { ...savingDocument, [kind]: true };
    documentError = { ...documentError, [kind]: null };
    try {
      const saved = await saveConfig(targetId, kind, {
        enabled: wanted,
        document,
        expected_revision: current.revision,
      });
      documents = { ...documents, [kind]: saved };
      plan = (await fetchPlan(targetId)).plan;

      return true;
    } catch (cause) {
      documentError = { ...documentError, [kind]: messageOf(cause) };

      return false;
    } finally {
      savingDocument = { ...savingDocument, [kind]: false };
    }
  }

  async function onApprove(planId: string, digest: string): Promise<void> {
    approving = true;
    error = null;
    try {
      plan = (await approvePlan(targetId, planId, digest)).plan;
    } catch (cause) {
      error = messageOf(cause);
    } finally {
      approving = false;
    }
  }

  /** The plan's three operations in the three words a plan is read with. */
  const VERB: Record<SyncAction['operation'], PlanOp> = {
    create: 'add',
    update: 'change',
    delete: 'remove',
  };

  function countOf(repository: string, operation: SyncAction['operation']): number {
    return actionsFor(repository).filter((action) => action.operation === operation).length;
  }

  /**
   * What would change about one thing, in the form a reader can check.
   *
   * `from → to` is the whole reason a change row exists; an addition says only
   * what arrives, and a removal says nothing at all, because the verb already
   * did.
   */
  function changeOf(action: SyncAction): string | undefined {
    if (
      action.operation === 'update' &&
      action.before !== undefined &&
      action.after !== undefined
    ) {
      return `${action.before} → ${action.after}`;
    }
    if (action.operation === 'create' && action.after !== undefined) return `— ${action.after}`;

    return undefined;
  }

  /** Why it did not happen, including the one that was never tried. */
  function failureOf(action: SyncAction): string | undefined {
    if (action.error !== undefined && action.error !== '') return action.error;
    if (action.blocker !== undefined && action.blocker !== '') {
      return `not tried: ${action.blocker} failed first`;
    }

    return undefined;
  }

  function messageOf(cause: unknown): string {
    return cause instanceof Error ? cause.message : String(cause);
  }

  const labels = $derived(config?.labels ?? []);
  const enabled = $derived(config?.enabled ?? false);

  /**
   * A configuration this version cannot read shows an empty list because
   * nothing came out of the stored document, not because nothing is
   * configured. Saving from that form would send the emptiness back and wipe a
   * label set nobody was ever shown, so nothing here is editable.
   */
  const unreadable = $derived(config?.unreadable === true);

  /**
   * What labels sync needs and this installation has not granted. Empty for
   * nearly every installation - labelling is what the bot was let in to do -
   * but the answer carries it for every kind, and a page that read it for one
   * kind and not the other would go quiet on whichever one was missed next.
   */
  const unavailable = $derived(config?.unavailable ?? '');

  /**
   * A plan is only worth approving while it is waiting for somebody. The other
   * states are reported rather than acted on, which is why the button is bound
   * to this and not merely to a plan existing.
   */
  const approvable = $derived(plan !== null && plan.state === 'computed');

  /** The strip of sections, as addresses: pressing one is a navigation. */
  const SECTION_LABELS: Record<SyncSection, string> = {
    overview: 'Overview',
    labels: 'Labels',
    settings: 'Settings',
    rulesets: 'Rulesets',
    files: 'Files',
    plan: 'Plan',
  };

  const waiting = $derived(plan?.actions.length ?? 0);

  /**
   * What the board can say, which is nothing until a plan has been worked out.
   *
   * A repository with no action in a computed plan is in step - that is what a
   * plan means. With no plan at all, the same repository has simply not been
   * looked at, and drawing it settled would be the panel answering a question
   * nobody has asked GitHub yet.
   */
  const boardReadable = $derived(plan !== null && fleet !== null);

  /**
   * The plan's actions, indexed the ways this page asks for them.
   *
   * Grouped once rather than scanned per question, and the page asks a lot of
   * them: the board asks twice per repository, each strip once per repository
   * per kind, and every ruleset and every shared file asks about its own
   * subject. At two hundred repositories and eight hundred actions that was on
   * the order of a million comparisons and a thousand throwaway arrays to draw
   * the page once.
   *
   * Nested rather than keyed on a joined string, because a separator has to be
   * a character neither half can hold and one half here is a file path, which
   * can hold any of them. `Map.groupBy` builds each level, so nothing here is a
   * Map a component writes into after building - which is worth avoiding on its
   * own, since that is state Svelte cannot see.
   */
  const planIndex = $derived.by(() => {
    const actions = plan?.actions ?? [];
    const byRepository = Map.groupBy(actions, (action) => action.repository);
    const byKind = Map.groupBy(actions, (action) => action.kind);

    return {
      byRepository,
      byRepositoryKind: new Map(
        [...byRepository].map(([repository, mine]) => [
          repository,
          Map.groupBy(mine, (action) => action.kind),
        ]),
      ),
      bySubject: new Map(
        [...byKind].map(([kind, mine]) => [kind, Map.groupBy(mine, (action) => action.subject)]),
      ),
    };
  });

  /** Shared, because every miss answers with one and none of them is written to. */
  const NO_ACTIONS: readonly SyncAction[] = [];

  /** Every action against one repository, from the plan on screen. */
  function actionsFor(repository: string, kind?: string): readonly SyncAction[] {
    const found =
      kind === undefined
        ? planIndex.byRepository.get(repository)
        : planIndex.byRepositoryKind.get(repository)?.get(kind);

    return found ?? NO_ACTIONS;
  }

  /**
   * What one repository is, from the plan and from whether sync watches it.
   *
   * `available` rather than `effective_enabled`: the second is the switch that
   * decides whether Smyklot answers commands on a repository, and sync does not
   * read it. What sync reads is `Available && !override.Disabled()` - see
   * `syncScope.watches` - and the override is per kind and not in this summary,
   * so a repository switched off for one kind alone still reads as settled
   * here. Saying more than that would be the panel inventing an answer.
   */
  function stateOf(repository: RepositorySummary, kind?: string): SyncState {
    if (!repository.available) return 'off';
    const mine = actionsFor(repository.name, kind);
    if (mine.some((action) => action.error !== undefined && action.error !== '')) return 'refused';

    return mine.length > 0 ? 'change' : 'settled';
  }

  const board = $derived.by((): BoardRepository[] =>
    (fleet?.items ?? []).map((repository) => {
      const state = stateOf(repository);
      const mine = actionsFor(repository.name);

      return {
        name: repository.name,
        state,
        changes: mine.length,
        reason: mine.find((action) => action.error !== undefined && action.error !== '')?.error,
      };
    }),
  );

  /**
   * What the plan would do about one named thing - a ruleset, a shared file.
   *
   * Nothing at all while there is no plan: a subject with no action in a
   * computed plan is in step, but with no plan the same subject has simply not
   * been looked at, and drawing a check would be the panel answering a question
   * nobody has asked GitHub yet.
   */
  function subjectMark(
    kind: string,
    subject: string,
  ): { state: SyncState; label?: string } | undefined {
    if (plan === null) return undefined;
    const mine = planIndex.bySubject.get(kind)?.get(subject) ?? NO_ACTIONS;
    const refused = mine.filter((action) => action.error !== undefined && action.error !== '');
    if (refused.length > 0) return { state: 'refused', label: 'refused' };
    if (mine.length === 0) return { state: 'settled' };

    const repositories = new Set(mine.map((action) => action.repository)).size;

    return {
      state: 'change',
      label: `${repositories} ${repositories === 1 ? 'repository differs' : 'repositories differ'}`,
    };
  }

  /** The same population, per kind, in the board's own order. */
  function strip(kind: string): SyncState[] {
    return (fleet?.items ?? []).map((repository) => stateOf(repository, kind));
  }

  /**
   * The repositories the plan names, gathered from the plan itself.
   *
   * Not from the board: the board draws one page of the fleet, and a plan can
   * name a repository that page does not hold. Counting out-of-step from the
   * tiles made the page contradict itself - five changes over two repositories,
   * beside a list of two carrying three between them.
   */
  /* Off the same grouping the rest of the page reads, rather than a fresh walk
     with `rows.find` inside it - which looked through every row gathered so far
     for every action in the plan. */
  const planRepositories = $derived(
    [...planIndex.byRepository].map(([name, mine]) => ({
      name,
      changes: mine.length,
      kinds: [...new Set(mine.map((action) => action.kind))],
      reason: mine.find((action) => action.error !== undefined && action.error !== '')?.error,
    })),
  );

  /** Every repository the installation has, not just the page on the board. */
  const population = $derived(fleet?.total ?? 0);

  /**
   * Who last changed one kind's configuration, and when.
   *
   * A configuration nobody has written yet says so rather than naming an empty
   * author - the four cards are read down a column, and a blank there reads as
   * a missing answer instead of an unmade decision.
   */
  function attribution(kind: SyncConfig | null): string | undefined {
    if (kind === null) return undefined;
    if (kind.updated_by === '') return 'never configured here';

    return `${kind.updated_by}, ${formatRelative(kind.updated_at, now)}`;
  }

  /** What the board is not showing, said rather than silently dropped. */
  const unshown = $derived(Math.max(population - (fleet?.items.length ?? 0), 0));

  const tabs = $derived(
    SYNC_SECTIONS.map((id) => ({
      id,
      label: SECTION_LABELS[id],
      href: sectionHref({ section: id }),
      // Only the plan carries a figure, and only when something is in it: a
      // count that waits on the reader is the one worth a colour.
      count: id === 'plan' && waiting > 0 ? String(waiting) : undefined,
      signal: id === 'plan',
    })),
  );

  /**
   * What one kind is configured to do, said in that kind's own terms.
   *
   * Read from the stored configuration rather than from the plan: the card
   * answers "what does this installation ask for", and the board beside it
   * already answers "and what would that change". A kind switched off says so
   * instead, because nothing it holds is being asked of anybody.
   */
  const summaries = $derived.by((): Record<string, string> => {
    const settings = documents.settings?.document ?? {};
    const managed = SETTING_KEYS.filter((key) => settings[key] !== undefined).length;
    const rulesets = storedList<SyncRuleset>(documents.rulesets?.document, 'rulesets');
    const evaluating = rulesets.filter((ruleset) => ruleset.enforcement === 'evaluate').length;
    const files = storedList<SyncFile>(documents.files?.document, 'files');
    const retired = storedList<string>(documents.files?.document, 'retired');

    return {
      labels: `${labels.length} ${labels.length === 1 ? 'label' : 'labels'} · removal ${config?.allow_removal === true ? 'on' : 'off'}`,
      settings: `${managed} of ${SETTING_KEYS.length} managed, the rest follow each repository`,
      rulesets:
        `${rulesets.length} ${rulesets.length === 1 ? 'ruleset' : 'rulesets'}` +
        (evaluating === 0 ? '' : ` · ${evaluating} evaluating`),
      files:
        `${files.length} ${files.length === 1 ? 'template' : 'templates'}` +
        (retired.length === 0
          ? ''
          : ` · ${retired.length} retired ${retired.length === 1 ? 'path' : 'paths'}`) +
        ' · changes arrive as pull requests',
    };
  });

  function kindSummary(kind: string, on: boolean): string {
    return on ? (summaries[kind] ?? '') : 'Off — nothing here is planned';
  }

  /**
   * The four cards on the overview, as data.
   *
   * Labels is not a document kind: it has an endpoint of its own, where the
   * switch and the rows share one revision. So it keeps its own entry and the
   * other three are the same entry three times - which is what makes the loop
   * worth having, since a card that grew a prop used to grow it four times and
   * get it right three.
   */
  const kindCards = $derived([
    {
      key: LABELS,
      name: 'Labels',
      on: enabled,
      when: attribution(config),
      toggle: (next: boolean) => void onSaveLabels(next),
    },
    ...(
      [
        [SETTINGS, 'Settings'],
        [RULESETS, 'Rulesets'],
        [FILES, 'Files'],
      ] as const
    ).map(([key, name]) => ({
      key,
      name,
      on: documents[key]?.enabled === true,
      when: attribution(documents[key]),
      toggle: (next: boolean) => void onSaveDocument(key, next, documents[key]?.document ?? {}),
    })),
  ]);

  /** The plan in one line, counting the removals out loud. */
  const planLine = $derived.by(() => {
    const repositories = planRepositories.length;
    const removals = plan?.counts.delete ?? 0;
    const line = `${waiting} ${waiting === 1 ? 'change' : 'changes'} across ${repositories} ${repositories === 1 ? 'repository' : 'repositories'}`;

    return removals === 0
      ? line
      : `${line}, including ${removals} ${removals === 1 ? 'removal' : 'removals'}`;
  });

  /** When it was worked out, how long it stands, and what it is waiting on. */
  const planWhen = $derived.by(() => {
    if (plan === null) return '';
    const parts = [`Worked out ${formatRelative(plan.computed_at, now)}`];
    if (plan.state === 'computed') parts.push(`expires ${formatUntil(plan.expires_at, now)}`);
    parts.push('nothing happens until you apply it');

    return parts.join(' · ');
  });

  /** How many of one repository's waiting changes take something away. */
  function removalsFor(repository: string): number {
    return actionsFor(repository).filter((action) => action.operation === 'delete').length;
  }

  const planNote = $derived(planExplanation(plan));

  /**
   * What a plan's state means, in the words somebody needs to decide what to do
   * next. "Stale" and "expired" are the pair worth spelling out: one says
   * somebody changed the configuration and the other says nobody came back in
   * time, and a reader who cannot tell them apart cannot tell whose turn it is.
   */
  function planExplanation(current: SyncPlan | null): string | null {
    if (current === null) {
      return null;
    }

    switch (current.state) {
      case 'computed':
        return 'Waiting for you. Nothing has been changed on GitHub yet.';
      case 'approved':
        return 'Approved, and waiting for the service to pick it up.';
      case 'applying':
        return 'Being applied now.';
      case 'applied':
        return 'Applied.';
      case 'failed':
        return 'Some of this did not apply. The rows below say which.';
      case 'stale':
        return 'The labels changed while this was on screen, so it no longer describes what would happen.';
      case 'expired':
        return 'Nobody acted on this in time. The next sweep will work it out again.';
      default:
        return null;
    }
  }
</script>

<section class="sync-page" aria-labelledby="sync-heading">
  <!-- One thing's own page carries its own head and a crumb back up to the list
       instead. The tab strip belongs to the level above it: repeating it here
       would offer a reader two ways up and name neither of them. -->
  {#if item === undefined}
    <PageHeader
      id="sync-heading"
      title="Sync"
      description="What every repository in this installation should look like, and what Smyklot would change to make that true"
    />

    <div class="sync-tabs">
      <SectionTabs items={tabs} active={section} label="Sync sections" />
    </div>
  {/if}

  {#if error !== null && section === 'overview'}
    <FormError message={error} />
  {/if}

  {#if section === 'overview'}
    {#if boardReadable}
      <!-- The verdict first, counted the way somebody arrives asking for it:
           how many need attention, not how many are fine. -->
      <div class="hero">
        <p class="hero-eyebrow band-trim">{account} · sync</p>
        <div>
          <h2 class="verdict-line band-trim">
            {#if planRepositories.length === 0}
              <strong>All {population}</strong> are in step
            {:else}
              <strong class="is-drift">{planRepositories.length} of {population}</strong>
              {planRepositories.length === 1 ? 'is' : 'are'} out of step
            {/if}
          </h2>
          <p class="hero-sub">
            What every repository here should look like, and what Smyklot would change to make that
            true. Nothing reaches GitHub until you approve a plan
          </p>
        </div>
        <!-- Right-aligned and quiet: the freshness of the answer, which decides
             how much of it to believe, without competing with the answer. -->
        <!-- Two short facts about the answer's freshness, which is what decides
             how much of it to believe. The mock's second line is the reconcile
             cadence; nothing in the API carries one, so this says how long the
             plan stands instead - the same shape, and true. -->
        <div class="hero-meta">
          {#if plan !== null}
            <span>Checked <strong>{formatRelative(plan.computed_at, now)}</strong></span>
            {#if plan.state === 'computed'}
              <span>Expires <strong>{formatUntil(plan.expires_at, now)}</strong></span>
            {/if}
          {/if}
        </div>
      </div>

      <SyncBoard
        repositories={board}
        label="Repositories in this installation"
        footLine={waiting === 0 ? undefined : planLine}
        footWhen={planWhen}
        hrefOf={repositoryHref === undefined
          ? undefined
          : (repository) => repositoryHref(repository.name)}
      >
        <!-- Filled, not tinted. `brand` is the bordered tone, and the approved
             mock draws this as a solid petrol button with white ink - which is
             exactly what `signal` already is, so the page takes the app's own
             filled tone rather than a new colour. -->
        <Button href={sectionHref({ section: 'plan' })} tone="signal">Review the plan</Button>
      </SyncBoard>

      {#if unshown > 0}
        <!-- Never a silent cap: a board that quietly drew the first hundred of
             four hundred would read as the whole fleet being in step. -->
        <p class="sync-lead sync-unshown">
          {unshown} more {unshown === 1 ? 'repository is' : 'repositories are'} not on the board
        </p>
      {/if}

      {#if planRepositories.length > 0}
        <div class="attn ruled-rows">
          {#each planRepositories as repository (repository.name)}
            {@const removals = removalsFor(repository.name)}
            <a
              class="attn-row"
              href={repositoryHref?.(repository.name) ?? sectionHref({ section: 'plan' })}
            >
              <span class="attn-repo">{repository.name}</span>
              <span class="attn-what">
                <StateMark
                  state={repository.reason !== undefined ? 'refused' : 'change'}
                  label={repository.reason !== undefined
                    ? 'refused'
                    : `${repository.changes} ${repository.changes === 1 ? 'change' : 'changes'}`}
                />
              </span>
              <!-- A refusal's reason belongs on its row. A state that blocks the
                   whole plan should never wait in a drill-down. -->
              <span class="attn-why" class:is-refused={repository.reason !== undefined}>
                {#if repository.reason !== undefined}
                  {repository.reason}
                {:else}
                  {repository.kinds.join(' · ')}{removals === 0
                    ? ''
                    : ` — ${removals} ${removals === 1 ? 'removal' : 'removals'} among them`}
                {/if}
              </span>
              <!-- The same chevron the kind cards carry, saying the same thing:
                   this row is a way somewhere, not a line of a report. -->
              <span class="attn-open" aria-hidden="true">
                <Icon name="chevron-right" size={12} />
              </span>
            </a>
          {/each}
        </div>
      {/if}
    {:else if fleet !== null}
      <p class="sync-lead">
        Nothing has been worked out yet. A reconcile runs on a timer and proposes whatever differs
      </p>
    {/if}

    <!-- What each kind is doing, and the way into it. Each strip repeats the
         board's slots in the board's order, so a repository that is out of step
         sits in the same column across all four.

         Every card's switch writes. Three of them were rendered without a
         handler and `SyncKindCard` calls `onToggle?.()`, so pressing one moved
         the thumb - a checkbox flips itself - and saved nothing, which told a
         reader they had just switched ruleset sync off across the whole
         installation. -->
    <div class="sync-kinds">
      {#each kindCards as card (card.key)}
        <SyncKindCard
          name={card.name}
          href={sectionHref({ section: card.key })}
          summary={kindSummary(card.key, card.on)}
          states={boardReadable ? strip(card.key) : undefined}
          when={card.when}
          enabled={card.on}
          onToggle={card.toggle}
        />
      {/each}
    </div>
  {/if}

  {#if section === 'labels'}
    <SyncLabelsForm
      {labels}
      allowRemoval={config?.allow_removal ?? false}
      excludes={config?.excludes ?? []}
      {enabled}
      {unreadable}
      {unavailable}
      problem={error}
      {readOnly}
      {saving}
      onSave={(wanted, wantedLabels, allowRemoval, excludes) =>
        void onSaveLabels(wanted, wantedLabels, allowRemoval, excludes)}
    />
  {/if}

  {#if section === 'settings' && documents.settings !== null}
    <SyncSettingsForm
      stored={documents.settings.document}
      enabled={documents.settings.enabled}
      unreadable={documents.settings.unreadable}
      unavailable={documents.settings.unavailable}
      problem={documentError.settings}
      {readOnly}
      saving={savingDocument.settings}
      onSave={(wanted, document) => onSaveDocument(SETTINGS, wanted, document)}
    />
  {/if}

  {#if section === 'rulesets' && documents.rulesets !== null}
    {#if item === undefined}
      <SyncRulesetsForm
        stored={documents.rulesets.document}
        enabled={documents.rulesets.enabled}
        unreadable={documents.rulesets.unreadable}
        unavailable={documents.rulesets.unavailable}
        problem={documentError.rulesets}
        {readOnly}
        saving={savingDocument.rulesets}
        rulesetHref={(name) => sectionHref({ section: 'rulesets', item: name })}
        markOf={(name) => subjectMark(RULESETS, name)}
        onSave={(wanted, document) => onSaveDocument(RULESETS, wanted, document)}
      />
    {:else}
      <SyncRulesetDetail
        stored={documents.rulesets.document}
        name={item}
        listHref={sectionHref({ section: 'rulesets' })}
        unreadable={documents.rulesets.unreadable}
        problem={documentError.rulesets}
        {readOnly}
        saving={savingDocument.rulesets}
        onSave={(document) =>
          onSaveDocument(RULESETS, documents.rulesets?.enabled === true, document)}
      />
    {/if}
  {/if}

  {#if section === 'files' && documents.files !== null}
    {#if item === undefined}
      <SyncFilesForm
        stored={documents.files.document}
        enabled={documents.files.enabled}
        unreadable={documents.files.unreadable}
        unavailable={documents.files.unavailable}
        problem={documentError.files}
        {readOnly}
        saving={savingDocument.files}
        fileHref={(path) => sectionHref({ section: 'files', item: path })}
        {adjustments}
        paths={known.paths}
        repositories={known.repositories === 0 ? population : known.repositories}
        pathsPartial={known.partial === true}
        {now}
        markOf={(path) => subjectMark(FILES, path)}
        onSave={(wanted, document) => onSaveDocument(FILES, wanted, document)}
      />
    {:else}
      <SyncFileDetail
        stored={documents.files.document}
        path={item}
        listHref={sectionHref({ section: 'files' })}
        {adjustments}
        repositories={population}
        updatedBy={documents.files.updated_by}
        updatedAt={documents.files.updated_at}
        {now}
        {readOnly}
        saving={savingDocument.files}
        unreadable={documents.files.unreadable}
        problem={documentError.files}
        onSave={(document) => onSaveDocument(FILES, documents.files?.enabled === true, document)}
        onSaveAdjustment={saveOverride === undefined
          ? undefined
          : (repositoryId, document) => onSaveAdjustment(repositoryId, document)}
      />
    {/if}
  {/if}

  {#if section === 'plan'}
    {#if plan === null}
      <!-- Deliberately not "nothing to do". Nothing is waiting, which is also
           what it looks like a moment after saving, before any reconcile has
           read a repository - and telling somebody their new configuration
           needs no changes would be a claim nothing here has checked. -->
      <p class="sync-lead">
        Nothing waiting. A reconcile runs on a timer and proposes whatever differs
      </p>
    {:else}
      <div class="plan-state">
        <p class="sync-lead">{planNote}</p>
        <p class="plan-counts">
          <span class="is-add">+{plan.counts.create} to add</span>
          <span class="is-change">~{plan.counts.update} to change</span>
          <span class="is-remove">−{plan.counts.delete} to remove</span>
        </p>
      </div>

      <!-- Grouped by the unit somebody is answerable for. The first group is
           open and the rest are folded, because their counts already say what
           is in them - which is what makes folding honest. -->
      {#each planRepositories as repository, at (repository.name)}
        <PlanGroup
          repository={repository.name}
          added={countOf(repository.name, 'create')}
          changed={countOf(repository.name, 'update')}
          removed={countOf(repository.name, 'delete')}
          open={at === 0}
        >
          {#each actionsFor(repository.name) as action (action.kind + action.subject)}
            <PlanAction
              op={VERB[action.operation]}
              kind={action.kind}
              what={action.subject}
              detail={changeOf(action)}
              failure={failureOf(action)}
            />
          {/each}
        </PlanGroup>
      {/each}

      {#if approvable && !readOnly}
        {@const approved = plan}
        <ApplyBar
          changes={plan.actions.length}
          repositories={planRepositories.length}
          removals={plan.counts.delete}
          asPullRequests={plan.actions.some((action) => action.kind === 'files')}
          applying={approving}
          onApply={() => void onApprove(approved.id, approved.digest)}
        />
      {/if}
    {/if}
  {/if}
</section>

<style>
  /* The settings page's plates, on the settings page's ground. */
  .sync-page :global(.plate) {
    background: var(--surface-base);
  }

  .sync-tabs {
    margin-bottom: var(--space-5);
  }

  /* The answer, at the size an answer is read. Two columns: the verdict, and
     how fresh it is - which is what decides how much of it to believe. */
  .hero {
    display: grid;
    gap: var(--space-3);
    grid-template-columns: 1fr auto;
    margin-block: var(--space-2) var(--space-5);
  }

  .hero-eyebrow {
    color: var(--text-muted);
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    font-weight: 500;
    grid-column: 1 / -1;
    letter-spacing: 0.08em;
    margin: 0;
    text-transform: uppercase;
  }

  .verdict-line {
    font-size: 2.35rem;
    font-weight: 700;
    letter-spacing: -0.03em;
    line-height: round(1.1em, 1px);
    margin: 0;
  }

  .is-drift {
    color: var(--drift);
  }

  .hero-sub {
    color: var(--text-secondary);
    font-size: var(--font-size-meta);
    margin: var(--space-3) 0 0;
    max-width: 56ch;
  }

  .hero-meta {
    align-self: end;
    color: var(--text-muted);
    display: grid;
    font-size: var(--font-size-micro);
    gap: var(--space-1);
    justify-items: end;
    text-align: end;
  }

  .hero-meta strong {
    color: var(--text-secondary);
    font-weight: 600;
  }

  @media (max-width: 52rem) {
    .hero {
      grid-template-columns: 1fr;
    }

    .hero-meta {
      justify-items: start;
      text-align: start;
    }

    .verdict-line {
      font-size: 1.9rem;
    }
  }

  .plan-state {
    display: grid;
    gap: var(--space-2);
    margin-bottom: var(--space-4);
  }

  /* The three verbs as figures, in the inks the rows below them use. */
  .plan-counts {
    display: flex;
    font-family: var(--mono);
    font-size: var(--font-size-compact);
    font-variant-numeric: tabular-nums;
    gap: var(--space-4);
    margin: 0;
  }

  .plan-counts .is-add {
    color: var(--diff-add-ink);
  }

  .plan-counts .is-change {
    color: var(--diff-chg-ink);
  }

  .plan-counts .is-remove {
    color: var(--diff-del-ink);
    font-weight: 600;
  }

  .sync-unshown {
    margin-top: var(--space-3);
  }

  /* The board in words: the same repositories, with the reason on the row. */
  .attn {
    display: grid;
    margin: var(--space-4) 0 var(--space-6);
  }

  /* The rule between two of these, and its manners around a hover, are
     `.ruled-rows` in app.css - the same idiom the file and ruleset lists use. */
  .attn-row {
    align-items: baseline;
    border-radius: var(--r-ctl);
    color: inherit;
    cursor: pointer;
    display: grid;
    gap: var(--space-3);
    grid-template-columns: 9.5rem auto 1fr auto;
    padding: 0.5rem var(--space-3);
    text-decoration: none;
  }

  /* Centred rather than on the row's baseline: the row aligns text to text, and
     a chevron has no baseline to give. */
  .attn-open {
    align-self: center;
    color: var(--text-muted);
    display: inline-flex;
    transition:
      color var(--duration-fast) var(--ease-standard),
      translate var(--duration-fast) var(--ease-standard);
  }

  /* It leans the way it would take the reader, and darkens as it goes. Two
     pixels: enough to be seen moving beside a name, not enough to read as the
     row itself shifting under the hand. */
  .attn-row:hover .attn-open {
    color: var(--text-primary);
    translate: 2px 0;
  }

  /* The layer rather than `--table-row-hover`, because this row rests on the
     page and not on a card. That token is an opaque colour mixed against
     `--surface-base`, so on the sunken ground it moved 1.44 dE00 in the light
     palette against the 2.5 every other row in the panel moves, and 7.6 in the
     dark one - too little and too much from the same declaration. A layer is
     composited over whatever is actually behind it and lands on 2.5 / 5.0
     either way. */
  .attn-row:hover {
    background-image: linear-gradient(
      var(--interactive-hover-layer),
      var(--interactive-hover-layer)
    );
  }

  .attn-row:active {
    background-image: linear-gradient(var(--press), var(--press));
    transform: scale(var(--press-scale-surface));
  }

  .attn-repo {
    font-family: var(--mono);
    font-size: var(--font-size-compact);
    font-weight: 500;
  }

  .attn-what {
    display: flex;
    gap: var(--space-2);
  }

  .attn-why {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
  }

  /* A refusal's words are the row's point, so they are not the quietest thing
     on it. */
  .attn-why.is-refused {
    color: var(--text-secondary);
  }

  @media (max-width: 52rem) {
    .attn-row {
      grid-template-columns: 1fr auto;
    }

    .attn-why {
      grid-column: 1 / -1;
    }
  }

  /* Four across where there is room, then two, then one. The cards are peers -
     no kind is the important one - so they share a row rather than stacking in
     an order that would imply one. */
  .sync-kinds {
    display: grid;
    gap: var(--space-4);
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }

  @media (max-width: 64rem) {
    .sync-kinds {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }

  @media (max-width: 40rem) {
    .sync-kinds {
      grid-template-columns: minmax(0, 1fr);
    }
  }

  :global(.form-error) {
    margin: var(--space-3) 0 0;
  }

  /* The colour still changes; only the lean goes. */
  @media (prefers-reduced-motion: reduce) {
    .attn-open {
      transition: none;
    }

    .attn-row:hover .attn-open {
      translate: none;
    }
  }
</style>
