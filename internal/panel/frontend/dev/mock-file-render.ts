import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process';
import { dirname, resolve } from 'node:path';
import { createInterface } from 'node:readline';
import { fileURLToPath } from 'node:url';

import {
  parseFormattingPolicy,
  parseFormattingSources,
  type FormattingPatch,
  type FormattingPolicy,
  type FormattingSources,
} from '../src/lib/formatting.ts';
import type {
  ConfigSource,
  SyncFileRenderDiagnostic,
} from '../src/lib/sync-file-render.generated.ts';
import type { SyncFileMerge } from '../src/lib/types.ts';

const VERSION = 1;
const MAX_MESSAGE_BYTES = 4 << 20;
const SOURCES: readonly ConfigSource[] = [
  'process',
  'target',
  'repository_file',
  'repository_panel',
  'template',
  'repository_path',
];

export interface GoRenderLayer {
  source: ConfigSource;
  formatting: FormattingPatch;
}

export interface GoRenderInput {
  path: string;
  draft_content: string;
  default_branch?: string;
  merge: Omit<SyncFileMerge, 'path'>;
  base_formatting: FormattingPolicy;
  layers: GoRenderLayer[];
  inherited_layers: number;
}

export interface GoRenderResponse {
  valid: boolean;
  final_content: string;
  matches_formatting: boolean;
  inherited_policy: FormattingPolicy;
  effective_policy: FormattingPolicy;
  provenance: FormattingSources<ConfigSource>;
  diagnostics: SyncFileRenderDiagnostic[];
}

type Pending = { resolve: (response: GoRenderResponse) => void; reject: (cause: Error) => void };

/** One persistent, bounded, shell-free Go renderer for the lifetime of Vite. */
export class GoFileRenderer {
  readonly #pending = new Map<string, Pending>();
  #process: ChildProcessWithoutNullStreams | null = null;
  #sequence = 0;

  render(input: GoRenderInput): Promise<GoRenderResponse> {
    const child = this.#runningProcess();
    const id = `render-${(this.#sequence += 1)}`;
    const message = `${JSON.stringify({ version: VERSION, id, ...input })}\n`;
    if (Buffer.byteLength(message) > MAX_MESSAGE_BYTES) {
      return Promise.reject(new Error('the development render request is too large'));
    }
    return new Promise((resolve, reject) => {
      this.#pending.set(id, { resolve, reject });
      child.stdin.write(message, (error) => {
        if (error === null || error === undefined) return;
        this.#pending.delete(id);
        reject(error);
      });
    });
  }

  close(): void {
    const child = this.#process;
    this.#process = null;
    if (child !== null) child.kill('SIGTERM');
    this.#rejectAll(new Error('the development renderer stopped'));
  }

  #runningProcess(): ChildProcessWithoutNullStreams {
    if (this.#process !== null) return this.#process;
    const executable =
      process.env.SMYKLOT_PANEL_RENDER_BRIDGE ??
      resolve(dirname(fileURLToPath(import.meta.url)), '../../../../bin/panel-render-bridge');
    const child = spawn(executable, [], { stdio: ['pipe', 'pipe', 'pipe'] });
    const lines = createInterface({ input: child.stdout });
    lines.on('line', (line) => this.#receive(line));
    child.stderr.setEncoding('utf8');
    child.stderr.on('data', (message: string) => console.error(message.trimEnd()));
    child.on('error', (error) => this.#fail(child, error));
    child.on('exit', (code, signal) => {
      this.#fail(
        child,
        new Error(`the development renderer exited (${code ?? signal ?? 'unknown'})`),
      );
    });
    this.#process = child;
    return child;
  }

  #receive(line: string): void {
    let value: unknown;
    try {
      value = JSON.parse(line) as unknown;
      const parsed = parseBridgeResponse(value);
      const id = bridgeID(value);
      const pending = this.#pending.get(id);
      if (pending === undefined) return;
      this.#pending.delete(id);
      pending.resolve(parsed);
    } catch (cause) {
      this.close();
      this.#rejectAll(cause instanceof Error ? cause : new Error(String(cause)));
    }
  }

  #fail(child: ChildProcessWithoutNullStreams, cause: Error): void {
    if (this.#process !== child) return;
    this.#process = null;
    this.#rejectAll(cause);
  }

  #rejectAll(cause: Error): void {
    for (const pending of this.#pending.values()) pending.reject(cause);
    this.#pending.clear();
  }
}

function parseBridgeResponse(value: unknown): GoRenderResponse {
  const record = exactRecord(value, [
    'version',
    'id',
    'valid',
    'final_content',
    'matches_formatting',
    'inherited_policy',
    'effective_policy',
    'provenance',
    'diagnostics',
  ]);
  const inherited = parseFormattingPolicy(record?.inherited_policy);
  const effective = parseFormattingPolicy(record?.effective_policy);
  const provenance = parseFormattingSources(record?.provenance, SOURCES);
  if (
    record === null ||
    record.version !== VERSION ||
    typeof record.id !== 'string' ||
    typeof record.valid !== 'boolean' ||
    typeof record.final_content !== 'string' ||
    typeof record.matches_formatting !== 'boolean' ||
    inherited === null ||
    effective === null ||
    provenance === null ||
    !Array.isArray(record.diagnostics)
  ) {
    throw new TypeError('the Go development renderer returned an invalid response');
  }
  return {
    valid: record.valid,
    final_content: record.final_content,
    matches_formatting: record.matches_formatting,
    inherited_policy: inherited,
    effective_policy: effective,
    provenance,
    diagnostics: record.diagnostics.map(parseDiagnostic),
  };
}

function parseDiagnostic(value: unknown): SyncFileRenderDiagnostic {
  const record = exactRecord(value, ['stage', 'code', 'message']);
  if (
    record === null ||
    typeof record.stage !== 'string' ||
    typeof record.code !== 'string' ||
    typeof record.message !== 'string'
  ) {
    throw new TypeError('the Go development renderer returned an invalid diagnostic');
  }
  return { stage: record.stage, code: record.code, message: record.message };
}

function bridgeID(value: unknown): string {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) return '';
  return typeof (value as Record<string, unknown>).id === 'string'
    ? ((value as Record<string, unknown>).id as string)
    : '';
}

function exactRecord(value: unknown, allowed: readonly string[]): Record<string, unknown> | null {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) return null;
  const record = value as Record<string, unknown>;
  return Object.keys(record).every((key) => allowed.includes(key)) ? record : null;
}
