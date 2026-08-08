/**
 * Tracks asynchronous reads that replace one shared view model. Only the most
 * recently started read may commit, so a slow older response cannot move the
 * UI back to stale data.
 */
export class LatestRequest {
  private generation = 0;

  begin(): number {
    this.generation += 1;
    return this.generation;
  }

  invalidate(): void {
    this.generation += 1;
  }

  isCurrent(generation: number): boolean {
    return generation === this.generation;
  }
}
