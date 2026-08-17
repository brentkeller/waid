import { randomInt } from 'node:crypto';

/**
 * Crockford base32: the digits plus the letters, less `i`, `l`, `o` and `u`, so an id read aloud or
 * copied by hand cannot be confused with another.
 */
export const ALPHABET = '0123456789abcdefghjkmnpqrstvwxyz';

/** Characters per item id. */
export const ID_LENGTH = 4;

/** Attempts before `newId` gives up, so a saturated or misbehaving `exists` cannot spin forever. */
const MAX_ATTEMPTS = 100;

/**
 * Ids pinned through `WAID_IDS`, consumed in order before randomness is reached for. The seam the
 * differential harness seeds so two processes writing the same log produce the same bytes.
 */
const pinned = (process.env.WAID_IDS ?? '').split(',').filter((id) => id !== '');

function randomId(): string {
  let id = '';
  for (let i = 0; i < ID_LENGTH; i += 1) {
    id += ALPHABET[randomInt(ALPHABET.length)];
  }
  return id;
}

/**
 * Generates an item id, retrying while `exists` reports the candidate as taken.
 *
 * @throws {Error} When no free id turns up within {@link MAX_ATTEMPTS}.
 */
export function newId(exists?: (id: string) => boolean): string {
  while (pinned.length > 0) {
    const id = pinned.shift() as string;
    if (!exists?.(id)) return id;
  }

  for (let attempt = 0; attempt < MAX_ATTEMPTS; attempt += 1) {
    const id = randomId();
    if (!exists?.(id)) return id;
  }
  throw new Error(`could not generate a unique id after ${MAX_ATTEMPTS} attempts`);
}
