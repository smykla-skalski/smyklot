export const SETTINGS_DRAFT_INACTIVITY_MINUTES = 30;
export const SETTINGS_DRAFT_INACTIVITY_MS = SETTINGS_DRAFT_INACTIVITY_MINUTES * 60 * 1_000;

export interface SettingsDraftAttentionState {
  dirty: boolean;
  lastChangedAt: number | null;
  attentionAt: number | null;
}

export interface SettingsDraftVisibilitySource {
  readonly visibilityState: DocumentVisibilityState;
  addEventListener(type: 'visibilitychange', listener: () => void): void;
  removeEventListener(type: 'visibilitychange', listener: () => void): void;
}

export interface SettingsDraftAttentionControllerOptions {
  now?: () => number;
  thresholdMs?: number;
  schedule?: (callback: () => void, delayMs: number) => ReturnType<typeof setTimeout>;
  cancel?: (handle: ReturnType<typeof setTimeout>) => void;
}

const cleanState: SettingsDraftAttentionState = {
  dirty: false,
  lastChangedAt: null,
  attentionAt: null,
};

/**
 * Turns a long-hidden dirty tab into one attention event.
 *
 * Browsers throttle background timers, so the visibility listener also checks
 * the elapsed time when the reader returns. A draft first received while the
 * tab is hidden gets its own full interval rather than inheriting the tab's age.
 */
export class SettingsDraftAttentionController {
  private state = cleanState;
  private hiddenAt: number | null;
  private timeout: ReturnType<typeof setTimeout> | null = null;
  private notified = false;
  private readonly now: () => number;
  private readonly thresholdMs: number;
  private readonly scheduleTimeout: NonNullable<
    SettingsDraftAttentionControllerOptions['schedule']
  >;
  private readonly cancelTimeout: NonNullable<SettingsDraftAttentionControllerOptions['cancel']>;

  constructor(
    private readonly source: SettingsDraftVisibilitySource,
    private readonly onAttention: () => void,
    options: SettingsDraftAttentionControllerOptions = {},
  ) {
    this.now = options.now ?? Date.now;
    this.thresholdMs = options.thresholdMs ?? SETTINGS_DRAFT_INACTIVITY_MS;
    this.scheduleTimeout =
      options.schedule ?? ((callback, delayMs) => setTimeout(callback, delayMs));
    this.cancelTimeout = options.cancel ?? clearTimeout;
    this.hiddenAt = source.visibilityState === 'hidden' ? this.now() : null;
    source.addEventListener('visibilitychange', this.onVisibilityChange);
  }

  update(state: SettingsDraftAttentionState): void {
    this.state = { ...state };
    if (!state.dirty) this.notified = false;
    this.reconcile();
  }

  dispose(): void {
    this.source.removeEventListener('visibilitychange', this.onVisibilityChange);
    this.cancelScheduledCheck();
  }

  private readonly onVisibilityChange = (): void => {
    if (this.source.visibilityState === 'hidden') {
      this.hiddenAt = this.now();
      this.reconcile();
      return;
    }

    // A background timeout may have been throttled. Check before forgetting how
    // long the tab was hidden, then cancel whichever timer is still pending.
    this.checkDue();
    this.hiddenAt = null;
    this.cancelScheduledCheck();
  };

  private reconcile(): void {
    this.cancelScheduledCheck();
    if (
      !this.state.dirty ||
      this.state.lastChangedAt === null ||
      this.state.attentionAt !== null ||
      this.notified ||
      this.source.visibilityState !== 'hidden'
    ) {
      return;
    }

    if (this.hiddenAt === null) this.hiddenAt = this.now();
    const delayMs = this.dueAt() - this.now();
    if (delayMs <= 0) {
      this.notify();
      return;
    }
    this.timeout = this.scheduleTimeout(this.onTimeout, delayMs);
  }

  private readonly onTimeout = (): void => {
    this.timeout = null;
    if (this.source.visibilityState !== 'hidden') return;
    if (!this.checkDue()) this.reconcile();
  };

  private checkDue(): boolean {
    if (
      !this.state.dirty ||
      this.state.lastChangedAt === null ||
      this.state.attentionAt !== null ||
      this.notified ||
      this.hiddenAt === null ||
      this.now() < this.dueAt()
    ) {
      return false;
    }
    this.notify();
    return true;
  }

  private dueAt(): number {
    return Math.max(this.hiddenAt ?? this.now(), this.state.lastChangedAt ?? 0) + this.thresholdMs;
  }

  private notify(): void {
    this.notified = true;
    this.cancelScheduledCheck();
    this.onAttention();
  }

  private cancelScheduledCheck(): void {
    if (this.timeout === null) return;
    this.cancelTimeout(this.timeout);
    this.timeout = null;
  }
}
