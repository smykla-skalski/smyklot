import { describe, expect, it } from 'vitest';

import { pairingHref } from '../src/lib/pairing';

describe('pairingHref', () => {
  it('passes a Harness pairing link through unchanged', () => {
    const link = 'harness://pair?payload=eyJjb2RlIjoiYWJjIn0';
    expect(pairingHref(link)).toBe(link);
  });

  // The scheme arrives from the daemon rather than from the page, and browsers
  // compare it case-insensitively, so the panel has to as well.
  it('accepts the scheme however it is spelled', () => {
    expect(pairingHref('HARNESS://pair?payload=abc')).toBe('HARNESS://pair?payload=abc');
  });

  // This value becomes an href. A scheme the browser executes in the panel's own
  // origin must never reach one, whatever minted it.
  it('refuses a scheme the browser would run', () => {
    expect(pairingHref('javascript:alert(1)')).toBeNull();
    expect(pairingHref('data:text/html,<script>alert(1)</script>')).toBeNull();
    expect(pairingHref('vbscript:msgbox(1)')).toBeNull();
  });

  it('refuses a scheme that is not the one the apps register', () => {
    expect(pairingHref('https://example.com/pair')).toBeNull();
    expect(pairingHref('harnessx://pair')).toBeNull();
    expect(pairingHref('harness-monitor://pair')).toBeNull();
  });

  it('refuses something that is not a URL at all', () => {
    expect(pairingHref('')).toBeNull();
    expect(pairingHref('pair?payload=abc')).toBeNull();
    expect(pairingHref('   ')).toBeNull();
  });
});
