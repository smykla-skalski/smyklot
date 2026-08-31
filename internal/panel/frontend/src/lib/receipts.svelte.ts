/**
 * WHAT JUST HAPPENED, AND THE WAY BACK.
 *
 * Every mutation in the product says what it did, in the words the page uses for the
 * thing it did it to - "Removed release-please - the next plan deletes it in all 25
 * syncing repositories" - and where the change can be taken back, the receipt is where
 * that is offered. Before this, a change to a list simply happened: the row left, and a
 * reader who pressed the wrong × had to know what had been there.
 *
 * One receipt is on screen at a time. A timed one yields the floor to whatever arrives
 * next; a sticky one keeps it, and the newcomer waits in line - a receipt somebody may
 * still press Undo on is not something the next receipt may take away from them.
 */
export interface Receipt {
  /** The sentence, which names the thing rather than the operation. */
  say: string;
  /** What pressing Undo does, where the change can be taken back. */
  undo?: () => void;
  /** Stays until it is dismissed or undone. For a change with a deadline behind it. */
  sticky?: boolean;
}

class Receipts {
  current = $state<Receipt | null>(null);
  #queue: Receipt[] = [];

  /** Reports one change. Returns nothing: a receipt is told, never asked. */
  say(say: string, options: Omit<Receipt, 'say'> = {}): void {
    const receipt = { say, ...options };
    if (this.current !== null && this.current.sticky === true) {
      this.#queue.push(receipt);
      return;
    }
    this.current = receipt;
  }

  /** Takes the current receipt off, and lets whatever was waiting have the floor. */
  dismiss(): void {
    this.current = this.#queue.shift() ?? null;
  }

  /** Presses Undo: the receipt goes, then the change is taken back. */
  undo(): void {
    const held = this.current?.undo;
    this.dismiss();
    held?.();
  }

  /** Empties the line. For a page leaving, and for a test starting. */
  clear(): void {
    this.#queue = [];
    this.current = null;
  }
}

export const receipts = new Receipts();
