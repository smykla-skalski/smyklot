import { describe, expect, it } from 'vitest';

import { createRequestRate, floodMessage } from '../src/lib/request-rate';

/**
 * The second half of the guard in `tests/effect-cycles`.
 *
 * That one keeps the shape that caused two shipped request storms out of the
 * source. This one notices a storm however it was caused, so a loop nobody
 * predicted still says so rather than running quietly at 1600 requests a second.
 */

function clock(): { at: () => number; advance: (ms: number) => void } {
  let time = 1_000;

  return {
    at: () => time,
    advance: (ms: number) => {
      time += ms;
    },
  };
}

describe('request rate [Unit]', () => {
  it('says nothing about a page doing its work', () => {
    const time = clock();
    const rate = createRequestRate(time.at);
    const floods = [
      '/api/v1/session',
      '/api/v1/targets',
      '/api/v1/notifications?limit=1',
      '/api/v1/root/overview',
    ].map((address) => rate.record(address));

    expect(floods).toEqual([null, null, null, null]);
  });

  it('says nothing about a burst that varies, however fast it types', () => {
    const time = clock();
    const rate = createRequestRate(time.at);
    for (let key = 0; key < 200; key += 1) {
      time.advance(10);
      expect(
        rate.record(`/api/v1/targets/1/user-suggestions?q=${'a'.repeat(key % 12)}`),
      ).toBeNull();
    }
  });

  it('names the address a loop is stuck on', () => {
    const time = clock();
    const rate = createRequestRate(time.at);
    let flood = null;
    for (let attempt = 0; attempt < 40 && flood === null; attempt += 1) {
      time.advance(1);
      flood = rate.record('/api/v1/notifications?limit=20');
    }

    expect(flood).toEqual({
      address: '/api/v1/notifications?limit=20',
      count: 26,
      withinMs: 2000,
    });
    expect(floodMessage(flood!)).toContain('/api/v1/notifications?limit=20');
    expect(floodMessage(flood!)).toContain('untrack');
  });

  /* The same address every few seconds is a page being used, not a loop, so the
     window has to forget rather than accumulate. */
  it('forgets what fell out of the window', () => {
    const time = clock();
    const rate = createRequestRate(time.at);
    for (let visit = 0; visit < 100; visit += 1) {
      time.advance(2001);
      expect(rate.record('/api/v1/targets')).toBeNull();
    }
  });

  it('keeps no record of an address nobody is asking for any more', () => {
    const time = clock();
    const rate = createRequestRate(time.at);
    for (let index = 0; index < 500; index += 1) {
      time.advance(100);
      rate.record(`/api/v1/repositories/${index}`);
    }
    // Whatever is left, one address at this rate must still read as ordinary.
    time.advance(5000);
    expect(rate.record('/api/v1/repositories/0')).toBeNull();
  });
});
