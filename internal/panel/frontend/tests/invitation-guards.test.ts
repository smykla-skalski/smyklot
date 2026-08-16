import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

/**
 * What the two invitation dialogs must not lose.
 *
 * The rules themselves belong to the server - it is the only place that knows the whole history,
 * and the only place a check cannot be raced by a second manager pressing at the same time. These
 * are the panel's half: the one standing it can answer before the press, and the two answers it
 * has to act on rather than print.
 *
 * Checked as source because the runtime here has no DOM. Narrow on purpose: each assertion names a
 * thing that would silently stop working, not a shape the markup happens to have today.
 */

const components = new URL('../src/lib/components/', import.meta.url);
const source = (file: string): string => readFileSync(new URL(file, components), 'utf8');

const dialogs = [
  ['UserManagement.svelte', 'createTargetInvitation'],
  ['RootInvitations.svelte', 'create({'],
] as const;

describe.each(dialogs)('%s', (file, createCall) => {
  const text = source(file);

  it('refuses the signed-in login before sending', () => {
    // A refusal the panel can make for itself, so naming yourself never costs a round trip. The
    // comparison has to fold case: GitHub logins are matched case-insensitively.
    expect(text).toMatch(/const namingSelf = \$derived\(/u);
    expect(text).toMatch(
      /login\.trim\(\)\.toLowerCase\(\) === actorLogin\.trim\(\)\.toLowerCase\(\)/u,
    );
  });

  it('keeps the submit control out of reach while it does', () => {
    expect(text).toMatch(/disabled=\{[^}]*namingSelf\}/u);
  });

  it('asks rather than reporting when the identity declined', () => {
    // The code is the contract with the server: printing the message instead would leave the
    // manager with a refusal and no way to answer it.
    expect(text).toContain("error.code === 'invitation_declined'");
    expect(text).toMatch(/declinedLogin = /u);
  });

  it('carries the acknowledgement only on the second attempt', () => {
    // Spread on a flag rather than always sending `false`, so a first attempt cannot pass the gate
    // by accident if the field is ever defaulted the other way round.
    expect(text).toMatch(/\.\.\.\(acknowledged \? \{ acknowledge_declined: true \} : \{\}\)/u);
    // A literal substring, asserted as one. Building a pattern from it meant escaping it by hand,
    // and a hand-rolled escape that misses the escape character is how that goes wrong.
    expect(text).toContain(createCall);
  });

  it('sends the first attempt without it', () => {
    expect(text).toMatch(/submitCreate|submitAdd/u);
    expect(text).toMatch(/(?:grantAccess|sendInvitation)\(false\)/u);
  });
});
