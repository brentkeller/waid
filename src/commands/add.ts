import { listFlag, stringFlag } from '../args.ts';
import { UserError } from '../errors.ts';
import { newId } from '../ids.ts';
import { knownProjects, resolveProject } from '../projects.ts';
import { appendEvent, loadState } from '../store.ts';
import type { CommandModule, Ctx, Status } from '../types.ts';

/** The item as recorded, echoed back so `--json` callers need no follow-up read. */
export type AddResult = {
  id: string;
  title: string;
  status: Status;
  project: string | null;
  tags: string[];
  waitingOn: string | null;
  session: string | null;
  created: string;
};

async function run(ctx: Ctx): Promise<AddResult> {
  const title = ctx.args.join(' ').trim();
  if (title === '') throw new UserError('add requires a title');

  const state = loadState(ctx.cfg);
  const project = resolveProject(
    stringFlag(ctx.flags, 'project'),
    knownProjects(state, ctx.sessions),
    ctx.cwd,
  );

  const waitingOn = stringFlag(ctx.flags, 'waiting-on') ?? null;
  const status: Status = waitingOn === null ? 'open' : 'waiting';
  const tags = listFlag(ctx.flags, 'tag');
  const session = stringFlag(ctx.flags, 'session') ?? null;

  const taken = new Set(state.items.map((item) => item.id));
  const id = newId((candidate) => taken.has(candidate));

  const event = appendEvent(ctx.cfg, {
    ev: 'add',
    id,
    title,
    status,
    project,
    session,
    tags,
    waitingOn,
  });

  return { id, title, status, project, tags, waitingOn, session, created: event.ts };
}

function render(data: AddResult): string {
  return `added ${data.id}  ${data.title}`;
}

export const add: CommandModule<AddResult> = { run, render };
