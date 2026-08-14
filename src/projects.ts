import { UserError } from './errors.ts';
import type { Item, ProjectGroup, State } from './types.ts';

export type { ProjectGroup };

/** A bare filesystem root — `/`, `C:` or `C:\` — where the trailing separator carries meaning. */
const BARE_ROOT = /^(?:[\\/]+|[A-Za-z]:[\\/]*)$/;

/** Windows drive-letter or POSIX absolute path. */
const ABSOLUTE = /^(?:[\\/]|[A-Za-z]:[\\/])/;

/** Trims a project path and drops trailing separators, leaving bare roots intact. */
export function normalizePath(p: string): string {
  const trimmed = p.trim();
  if (BARE_ROOT.test(trimmed)) return trimmed;
  return trimmed.replace(/[\\/]+$/, '');
}

/**
 * Every project path waid has seen, in first-seen order: items first, then sessions. Sessions stay
 * `unknown` so this module does not depend on the session shape.
 */
export function knownProjects(
  state: Pick<State, 'items'>,
  sessions: readonly unknown[] = [],
): string[] {
  const known: string[] = [];
  const add = (project: unknown): void => {
    if (typeof project !== 'string' || project === '') return;
    if (!known.includes(project)) known.push(project);
  };

  for (const item of state.items) add(item.project);
  for (const session of sessions) {
    if (typeof session === 'object' && session !== null && 'project' in session) {
      add((session as { project: unknown }).project);
    }
  }
  return known;
}

/**
 * Resolves a `-p` value: nothing → no project, `.` → the current directory, an absolute path taken
 * as given, anything else a case-insensitive substring of a known project path.
 *
 * @throws {UserError} When a partial matches no known project or more than one.
 */
export function resolveProject(
  input: string | undefined,
  known: string[],
  cwd: string,
): string | null {
  const value = input?.trim() ?? '';
  if (value === '') return null;
  if (value === '.') return normalizePath(cwd);
  if (ABSOLUTE.test(value)) return normalizePath(value);

  const needle = value.toLowerCase();
  const matches = known.filter((project) => project.toLowerCase().includes(needle));
  if (matches.length === 1) return matches[0] ?? null;
  if (matches.length === 0) {
    throw new UserError(`no known project matches: ${value}`);
  }
  throw new UserError(`ambiguous project: ${value}`, { candidates: matches });
}

/**
 * Groups items by project in first-seen order, with the project-less group last. Item order inside
 * each group is the order given.
 */
export function groupByProject(items: readonly Item[]): ProjectGroup[] {
  const groups = new Map<string, ProjectGroup>();
  const orphans: ProjectGroup = { project: null, items: [] };

  for (const item of items) {
    if (item.project === null) {
      orphans.items.push(item);
      continue;
    }
    const group = groups.get(item.project) ?? { project: item.project, items: [] };
    group.items.push(item);
    groups.set(item.project, group);
  }

  const ordered = [...groups.values()];
  if (orphans.items.length > 0) ordered.push(orphans);
  return ordered;
}
