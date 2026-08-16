import { readFileSync } from 'node:fs';

/**
 * The four palettes a control actually renders in, reconstructed from `app.css`.
 *
 * The panel has two themes and the Root console re-skins both of them, so every rule about a
 * hover, a press or a selected fill has four answers, not one. A number checked only against the
 * light panel is a number checked against a quarter of the product - and the Root shell is the
 * quarter most likely to drift, because it overrides the surfaces without overriding everything
 * derived from them.
 *
 * Custom properties substitute at computed-value time on the element that declares them, which is
 * why this resolves per element rather than by flattening every block into one map: `--accent:
 * var(--brand-action)` declared on `:root` is already resolved to petrol by the time it inherits
 * into the violet Root shell. That is a real cascade behaviour the shell works around by
 * re-declaring its aliases, and a resolver that ignored it would report colours nobody sees.
 */

const css = readFileSync(new URL('../src/app.css', import.meta.url), 'utf8');

type Declarations = Record<string, string>;

function block(selector: string): Declarations {
  const start = css.indexOf(`\n${selector} {`);
  if (start === -1) throw new Error(`app.css has no \`${selector}\` block`);
  const open = css.indexOf('{', start);
  const end = css.indexOf('\n}', open);
  if (end === -1) throw new Error(`\`${selector}\` block is unterminated`);
  return Object.fromEntries(
    [...css.slice(open, end).matchAll(/--(?<name>[\w-]+):\s*(?<value>[^;]+);/gu)].map((entry) => [
      entry.groups?.name ?? '',
      (entry.groups?.value ?? '').replaceAll(/\s+/gu, ' ').trim(),
    ]),
  );
}

const layers = {
  base: block(':root'),
  dark: block(":root[data-theme='dark']"),
  rootMode: block('.app-shell.root-mode'),
  rootModeDark: block(":root[data-theme='dark'] .app-shell.root-mode"),
};

/**
 * Every `--x: var(--y)` the panel declares on `:root`, as the pair it promises to be.
 *
 * These are the declarations the substitution rule above can silently break: each one resolves on
 * `:root` against panel values and inherits into the Root shell already answered, so a shell that
 * overrides `--y` gets a stale `--x` unless it re-declares that too. Read from the stylesheet
 * rather than listed by hand, because a list is the thing that goes out of date - which is exactly
 * how five of these came to be wrong in the Root console.
 */
export const rootAliases: readonly (readonly [alias: string, source: string])[] = Object.entries(
  layers.base,
)
  .map(
    ([alias, value]) => [alias, /^var\(--(?<name>[\w-]+)\)$/u.exec(value)?.groups?.name] as const,
  )
  .filter((pair): pair is readonly [string, string] => pair[1] !== undefined);

/** A colour, plus the alpha it is painted at when the declaration is translucent. */
export interface Paint {
  readonly color: string;
  readonly alpha: number;
}

export interface Palette {
  readonly name: string;
  /** The opaque colour a token resolves to, with every `var()` and `color-mix()` carried out. */
  color: (token: string) => string;
  /** A token that may be translucent, as a colour plus the alpha it paints at. */
  paint: (token: string) => Paint;
  /** The declaration as written, for rules that need to read a shadow or a gradient. */
  declaration: (token: string) => string;
  /** Whether the palette declares the token at all. */
  has: (token: string) => boolean;
}

function hexOf(value: string): string | undefined {
  const six = value.match(/^#(?<digits>[\da-f]{6})$/iu);
  if (six !== null) return `#${(six.groups?.digits ?? '').toLowerCase()}`;
  const three = value.match(/^#(?<digits>[\da-f]{3})$/iu);
  if (three === null) return undefined;
  return `#${[...(three.groups?.digits ?? '')].map((digit) => digit + digit).join('')}`.toLowerCase();
}

function rgbOf(value: string): Paint | undefined {
  const match = value.match(
    /^rgba?\(\s*(?<r>\d+)[\s,]+(?<g>\d+)[\s,]+(?<b>\d+)\s*(?:[/,]\s*(?<a>[\d.]+)(?<pct>%?))?\s*\)$/u,
  );
  if (match === null) return undefined;
  const channel = (raw: string | undefined): string =>
    Number(raw ?? 0)
      .toString(16)
      .padStart(2, '0');
  const raw = match.groups?.a;
  const alpha = raw === undefined ? 1 : match.groups?.pct === '%' ? Number(raw) / 100 : Number(raw);
  return {
    color: `#${channel(match.groups?.r)}${channel(match.groups?.g)}${channel(match.groups?.b)}`,
    alpha,
  };
}

/**
 * One element's computed custom properties: its own declarations win over what it inherits, and a
 * `var()` inside them sees the element's own values first.
 */
function element(own: Declarations, inherited: Map<string, Paint>): Map<string, Paint> {
  const computed = new Map(inherited);
  const resolving = new Set<string>();

  function value(raw: string): Paint {
    const trimmed = raw.trim();

    const hex = hexOf(trimmed);
    if (hex !== undefined) return { color: hex, alpha: 1 };

    const rgb = rgbOf(trimmed);
    if (rgb !== undefined) return rgb;

    const reference = trimmed.match(/^var\(--(?<name>[\w-]+)\)$/u)?.groups?.name;
    if (reference !== undefined) return token(reference);

    const blend = trimmed.match(
      /^color-mix\(in srgb,\s*(?<top>.+?)\s+(?<share>[\d.]+)%,\s*(?<base>.+)\)$/u,
    );
    if (blend !== null) {
      const top = value(blend.groups?.top ?? '');
      const base = value(blend.groups?.base ?? '');
      const share = Number(blend.groups?.share) / 100;
      const channels = (color: string): number[] =>
        [1, 3, 5].map((offset) => Number.parseInt(color.slice(offset, offset + 2), 16));
      const [topChannels, baseChannels] = [channels(top.color), channels(base.color)];
      const mixed = topChannels.map(
        (channel, index) => channel * share + (baseChannels[index] ?? 0) * (1 - share),
      );
      return {
        color: `#${mixed.map((channel) => Math.round(channel).toString(16).padStart(2, '0')).join('')}`,
        alpha: top.alpha * share + base.alpha * (1 - share),
      };
    }

    if (trimmed === 'black') return { color: '#000000', alpha: 1 };
    if (trimmed === 'white') return { color: '#ffffff', alpha: 1 };
    if (trimmed === 'transparent') return { color: '#000000', alpha: 0 };

    throw new Error(`cannot resolve \`${trimmed}\` to a colour`);
  }

  function token(name: string): Paint {
    if (resolving.has(name)) throw new Error(`--${name} resolves in a circle`);
    const declared = own[name];
    if (declared === undefined) {
      const already = computed.get(name);
      if (already === undefined) throw new Error(`no palette declares --${name}`);
      return already;
    }
    resolving.add(name);
    try {
      const resolved = value(declared);
      computed.set(name, resolved);
      return resolved;
    } finally {
      resolving.delete(name);
    }
  }

  for (const name of Object.keys(own)) {
    // A non-colour token (a duration, a radius, a shadow) has no business being resolved here;
    // `declaration` reads those as written.
    try {
      token(name);
    } catch {
      computed.delete(name);
    }
  }
  return computed;
}

function palette(name: string, html: Declarations[], shell: Declarations[]): Palette {
  const merge = (blocks: Declarations[]): Declarations => Object.assign({}, ...blocks);
  const htmlOwn = merge(html);
  const shellOwn = merge(shell);
  const computed = element(shellOwn, element(htmlOwn, new Map()));
  const written: Declarations = { ...htmlOwn, ...shellOwn };

  const paint = (token: string): Paint => {
    const found = computed.get(token);
    if (found === undefined) throw new Error(`${name} has no colour for --${token}`);
    return found;
  };

  return {
    name,
    paint,
    color: (token) => paint(token).color,
    declaration: (token) => {
      const value = written[token];
      if (value === undefined) throw new Error(`${name} does not declare --${token}`);
      return value;
    },
    has: (token) => written[token] !== undefined,
  };
}

/**
 * Every combination a control renders in. The Root shell inherits the panel's `:root` first, so a
 * token it does not override reaches it already resolved against panel surfaces - which is how a
 * petrol thumb ends up on a violet track unless someone checks.
 */
export const palettes: readonly Palette[] = [
  palette('panel light', [layers.base], []),
  palette('panel dark', [layers.base, layers.dark], []),
  palette('root light', [layers.base], [layers.rootMode]),
  palette('root dark', [layers.base, layers.dark], [layers.rootMode, layers.rootModeDark]),
];

export const [panelLight, panelDark, rootLight, rootDark] = palettes;

/** The share a `color-mix()` in a component's own stylesheet blends at, read from the file. */
export function mixShare(file: string, pattern: RegExp): number {
  const source = readFileSync(new URL(`../src/${file}`, import.meta.url), 'utf8');
  const match = source.match(pattern);
  if (match?.groups?.share === undefined) {
    throw new Error(`src/${file} no longer matches ${pattern}`);
  }
  return Number(match.groups.share) / 100;
}
