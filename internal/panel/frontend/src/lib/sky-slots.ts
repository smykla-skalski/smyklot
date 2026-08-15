/**
 * How many easter eggs may be on the sky at once.
 *
 * The rule is two. The rocket, which flies whenever its home is active,
 * holds its seat from the moment it enters until the last of its trail has
 * faded; the occasional visitors - the astronaut, a meteor shower - ask for
 * a seat when their moment comes and simply try again later if none is
 * free. Nothing queues and nothing is owed: a refusal costs a visitor one
 * rescheduled timer.
 *
 * One instance governs one page. The light theme's sky band and the dark
 * theme's page overlay share it, which is what keeps the cap honest across
 * a theme switch: a rocket still leaving the old home keeps its seat until
 * its trail has faded. When the sky was otherwise empty the new home's
 * rocket may take the spare seat and enter while the old one is still on
 * its way out - two on screen, which is exactly the budget - and when a
 * visitor holds that seat, the new rocket waits its turn.
 */
export class SkySlots {
  private used = 0;

  constructor(private readonly capacity = 2) {}

  /** Claim a seat; false means the sky is full and the caller waits. */
  take(): boolean {
    if (this.used >= this.capacity) return false;
    this.used += 1;
    return true;
  }

  release(): void {
    this.used = Math.max(0, this.used - 1);
  }
}
