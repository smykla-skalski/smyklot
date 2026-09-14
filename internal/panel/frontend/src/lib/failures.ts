import { sentenceCase } from './format';

/** Failure history contains terminal deliveries, not attempts waiting in the queue. */
export function failureClassification(retryable: boolean): {
  label: string;
  tone: 'warning' | 'danger';
  guidance: string;
} {
  return retryable
    ? {
        label: 'May succeed on retry',
        tone: 'warning',
        guidance: 'Automatic retries have stopped',
      }
    : {
        label: 'Needs a fix',
        tone: 'danger',
        guidance: 'Fix the cause before retrying',
      };
}

/**
 * What was being attempted, said as the act rather than as the lane it failed in.
 *
 * The wire carries the stage a delivery died at - `decode`, `execute`, and the
 * two the bot names for itself - and a row that read "Execute smyklot" told a
 * reader which branch of the code they were in. Each phrase ends where the
 * repository begins, so a row reads as one sentence.
 */
const STAGE_ACTS: Record<string, string> = {
  decode: 'Read the delivery for',
  execute: 'Run the command in',
  config: 'Read the repository file of',
  github: 'Ask GitHub about',
};

export function failureAct(stage: string): string {
  return STAGE_ACTS[stage] ?? sentenceCase(stage);
}
