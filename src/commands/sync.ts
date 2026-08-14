import { syncSessions } from '../sessions.ts';
import type { CommandModule, Ctx } from '../types.ts';

/** What a sync moved, so a run is legible without diffing the cache file. */
export type SyncResult = {
  /** Transcripts found on disk. */
  scanned: number;
  /** Transcripts re-read because they were new or had changed. */
  parsed: number;
  /** Transcripts skipped because their stats matched the cache. */
  reused: number;
  /** Cached sessions dropped because their transcript is gone. */
  removed: number;
  /** Sessions in the cache afterwards. */
  sessions: number;
  syncedAt: string;
};

async function run(ctx: Ctx): Promise<SyncResult> {
  const result = syncSessions(ctx.cfg, { full: ctx.flags.full === true, now: ctx.now });

  return {
    ...result.stats,
    sessions: result.sessions.length,
    syncedAt: result.syncedAt,
  };
}

function render(data: SyncResult): string {
  const detail = `parsed ${data.parsed}, reused ${data.reused}, removed ${data.removed}`;
  return `synced ${data.sessions} session${data.sessions === 1 ? '' : 's'}  ${detail}`;
}

export const sync: CommandModule<SyncResult> = { run, render };
