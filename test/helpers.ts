import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

import type { Deps } from '../src/cli.ts';
import { run } from '../src/cli.ts';

/** A fresh temp home. Sets `WAID_SKIP_GH_DETECT` so `ensureHome` never shells out to `gh`. */
export function makeHome(): string {
  process.env.WAID_SKIP_GH_DETECT = '1';
  return fs.mkdtempSync(path.join(os.tmpdir(), 'waid-cli-'));
}

export type CliResult = {
  code: number;
  out: string;
  err: string;
  /** Parses whichever stream carried output — stdout for results, stderr for `--json` errors. */
  json: () => unknown;
};

/** Runs the CLI in-process against `home`, capturing both streams. */
export async function waid(home: string, argv: string[], deps: Deps = {}): Promise<CliResult> {
  let out = '';
  let err = '';
  const code = await run(
    ['--waid-home', home, ...argv],
    {
      out: (text) => {
        out += text;
      },
      err: (text) => {
        err += text;
      },
    },
    deps,
  );
  return {
    code,
    out,
    err,
    json: () => JSON.parse(out.trim() || err.trim()),
  };
}
