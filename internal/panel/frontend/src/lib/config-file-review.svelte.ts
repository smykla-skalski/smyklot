import { PanelApiError } from './api';
import type {
  ConfigFileChoiceSide,
  ConfigFilePreview,
  ConfigFileResolutionInput,
  ConfigFileResolutionReceipt,
} from './config-file-sync';

export interface ConfigurationReviewClient {
  preview: (targetId: string, repositoryId?: string) => Promise<ConfigFilePreview>;
  resolve: (
    targetId: string,
    input: ConfigFileResolutionInput,
    repositoryId?: string,
  ) => Promise<ConfigFileResolutionReceipt>;
}

/** The owner supplies current authority and scope, never a draft document to write. */
export interface ConfigurationReviewSource {
  identity: string;
  prepare?: () => void;
  hasDrafts: boolean;
  canWrite: boolean;
  enabled: boolean;
  fileIgnored: boolean;
  preview: () => Promise<ConfigFilePreview>;
  resolve: (input: ConfigFileResolutionInput) => Promise<ConfigFileResolutionReceipt>;
  onResolved: () => void;
}

export class ConfigurationReview {
  open = $state(false);
  loading = $state(false);
  saving = $state(false);
  pending = $state(false);
  preview = $state.raw<ConfigFilePreview | null>(null);
  selected = $state<ConfigFileChoiceSide | null>(null);
  problem = $state('');
  notice = $state('');
  #generation = 0;
  #identity = '';

  constructor(private readonly current: () => ConfigurationReviewSource | undefined) {}

  get blocker(): string {
    const source = this.current();
    if (!source) return 'Configuration file review is unavailable';
    if (source.hasDrafts)
      return 'Save or discard this workspace’s changes before comparing saved settings';
    if (!source.enabled) return 'Save configuration file sync as on before reviewing changes';
    if (source.fileIgnored) return 'Turn on Use file settings and save before reviewing changes';
    return '';
  }

  get choice() {
    return (
      this.preview?.choices?.find(
        (choice) =>
          choice.side === this.selected && choice.available && choice.document !== undefined,
      ) ?? null
    );
  }

  get canSubmit(): boolean {
    return (
      this.open &&
      !this.loading &&
      !this.saving &&
      !this.pending &&
      !this.blocker &&
      this.current()?.identity === this.#identity &&
      this.current()?.canWrite === true &&
      !!this.preview?.review_token &&
      this.choice !== null
    );
  }

  /** Called reactively, while response guards also read current context synchronously. */
  reconcile(): void {
    if (!this.open) return;
    if (this.current()?.identity !== this.#identity) {
      this.close();
      return;
    }
    if (this.blocker) {
      this.#generation++;
      this.preview = null;
      this.selected = null;
      this.loading = false;
      this.saving = false;
    }
  }

  async show(): Promise<void> {
    this.open = true;
    this.#identity = this.current()?.identity ?? '';
    this.pending = false;
    this.notice = '';
    await this.refresh();
  }

  close(): void {
    this.#generation++;
    this.open = false;
    this.loading = false;
    this.saving = false;
    this.preview = null;
    this.selected = null;
  }

  private accepts(generation: number, identity: string): boolean {
    return (
      this.open &&
      generation === this.#generation &&
      this.current()?.identity === identity &&
      !this.blocker
    );
  }

  async refresh(): Promise<void> {
    this.current()?.prepare?.();
    this.preview = null;
    this.selected = null;
    this.problem = '';
    const source = this.current();
    if (!this.open || !source || this.blocker || this.pending) return;
    const generation = ++this.#generation;
    const identity = source.identity;
    this.#identity = identity;
    this.loading = true;
    try {
      const preview = await source.preview();
      if (this.accepts(generation, identity)) {
        this.preview = preview;
        if (
          preview.problem === 'file_removed' &&
          preview.choices?.length === 1 &&
          preview.choices[0]?.side === 'panel' &&
          preview.choices[0].available &&
          preview.choices[0].document !== undefined
        )
          this.selected = 'panel';
      }
    } catch (error) {
      if (this.accepts(generation, identity)) this.problem = message(error);
    } finally {
      if (generation === this.#generation) this.loading = false;
    }
  }

  select(side: string): void {
    if (this.loading || this.saving || this.pending || this.blocker) return;
    if (
      this.preview?.choices?.some(
        (choice) => choice.side === side && choice.available && choice.document !== undefined,
      )
    ) {
      this.selected = side as ConfigFileChoiceSide;
    }
  }

  async submit(): Promise<void> {
    this.current()?.prepare?.();
    if (!this.canSubmit) return;
    const source = this.current()!;
    const generation = ++this.#generation;
    const identity = source.identity;
    const input = { review_token: this.preview!.review_token!, side: this.selected! };
    this.saving = true;
    this.problem = '';
    try {
      await source.resolve(input);
      if (!this.accepts(generation, identity)) return;
      this.pending = true;
      this.selected = null;
      this.preview = null;
      source.onResolved();
    } catch (error) {
      if (!this.accepts(generation, identity)) return;
      this.preview = null;
      this.selected = null;
      if (error instanceof PanelApiError && error.status === 409) {
        this.notice = 'Settings or the file changed · review the new comparison and choose again';
        this.saving = false;
        await this.refresh();
      } else this.problem = message(error);
    } finally {
      if (generation === this.#generation) this.saving = false;
    }
  }
}

function message(error: unknown): string {
  return (error instanceof Error ? error.message : 'The comparison could not be completed').replace(
    /[.]+$/u,
    '',
  );
}
