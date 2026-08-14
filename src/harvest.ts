import type { Session } from './types.ts';

/** Characters of a user message kept as a fallback title. */
const TITLE_LENGTH = 80;

/**
 * Mutable state built up one transcript line at a time. Only the handful of fields the session
 * layer needs are tracked; the rest of each record is ignored.
 */
export type Accumulator = {
  id: string | null;
  aiTitle: string | null;
  firstMessage: string | null;
  project: string | null;
  branch: string | null;
  started: string | null;
  ended: string | null;
  prompts: number;
};

export function newAccumulator(): Accumulator {
  return {
    id: null,
    aiTitle: null,
    firstMessage: null,
    project: null,
    branch: null,
    started: null,
    ended: null,
    prompts: 0,
  };
}

/**
 * Applies one raw transcript line to the accumulator.
 *
 * Streaming and pure: no filesystem, no whole-file buffering, so a 2.5 MB transcript costs one line
 * of memory. Transcript records are third-party data, so a line that is not a JSON object — a blank
 * line, or the half-written final line of a session being appended to right now — is skipped
 * silently rather than treated as an error.
 */
export function feedLine(acc: Accumulator, rawLine: string): void {
  const line = rawLine.trim();
  if (line === '') return;

  let parsed: unknown;
  try {
    parsed = JSON.parse(line);
  } catch {
    return;
  }
  if (!isRecord(parsed)) return;

  const sessionId = str(parsed['sessionId']);
  if (acc.id === null && sessionId !== null) acc.id = sessionId;

  const timestamp = str(parsed['timestamp']);
  if (timestamp !== null) {
    // Every record type contributes to the span, so sidechain work keeps the session's real length.
    if (acc.started === null) acc.started = timestamp;
    acc.ended = timestamp;
  }

  const type = str(parsed['type']);

  if (type === 'ai-title') {
    // Emitted repeatedly as the title is refined; the last one wins.
    const aiTitle = str(parsed['aiTitle']);
    if (aiTitle !== null) acc.aiTitle = aiTitle;
    return;
  }

  if (type !== 'user') return;

  const sidechain = flag(parsed['isSidechain']);
  const meta = flag(parsed['isMeta']);

  if (!sidechain && acc.project === null) acc.project = str(parsed['cwd']);

  const branch = str(parsed['gitBranch']);
  if (branch !== null && branch !== '') acc.branch = branch;

  if (!meta && acc.firstMessage === null) {
    const text = messageText(parsed['message']);
    if (text !== null) acc.firstMessage = text;
  }

  if (!sidechain && !meta) acc.prompts += 1;
}

/**
 * Distils the accumulated state into a `Session`.
 *
 * `fallbackProject` is the caller's best guess from the transcript's directory name — lossy, and
 * used only when no user record carried a `cwd`.
 */
export function finalize(acc: Accumulator, opts: { fallbackProject: string | null }): Session {
  return {
    id: acc.id ?? '',
    title: acc.aiTitle ?? acc.firstMessage?.slice(0, TITLE_LENGTH) ?? '(untitled)',
    project: acc.project ?? opts.fallbackProject,
    branch: acc.branch,
    started: acc.started,
    ended: acc.ended,
    prompts: acc.prompts,
  };
}

/**
 * Best-effort reversal of a Claude Code project directory name back into a path.
 *
 * Explicitly lossy: the slug flattens both separators and literal dashes, so `C--dev-dr-devresults`
 * could be `C:\dev\dr\devresults` or `C:\dev\dr-devresults`. Only a last resort — a transcript's
 * `cwd` is always preferred.
 */
export function decodeProjectSlug(dirName: string): string {
  const windows = /^([A-Za-z])--(.*)$/.exec(dirName);
  if (windows !== null) {
    const [, drive = '', rest = ''] = windows;
    return `${drive.toUpperCase()}:\\${rest.replaceAll('-', '\\')}`;
  }
  if (dirName.startsWith('-')) return dirName.replaceAll('-', '/');
  return dirName;
}

/** Pulls the text out of a user message, whose content is either a string or content blocks. */
function messageText(message: unknown): string | null {
  if (!isRecord(message)) return null;
  const content = message['content'];
  if (typeof content === 'string') return content.trim() || null;
  if (!Array.isArray(content)) return null;

  for (const block of content) {
    if (!isRecord(block)) continue;
    const text = str(block['text']);
    if (text !== null && text.trim() !== '') return text.trim();
  }
  return null;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function str(value: unknown): string | null {
  return typeof value === 'string' ? value : null;
}

/** An absent `isSidechain`/`isMeta` counts as false, so any falsy value reads as "not set". */
function flag(value: unknown): boolean {
  return Boolean(value);
}
