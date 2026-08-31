import { sentenceCase } from './format';

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
