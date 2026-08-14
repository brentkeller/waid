/**
 * An error caused by the user's input or environment. The CLI maps it to exit code 1 and prints
 * only its message; every other error is internal and exits 2 with a stack.
 */
export class UserError extends Error {
  /** Alternatives to show when the input was ambiguous. */
  readonly candidates: string[] | null;

  constructor(message: string, opts?: { candidates?: string[] | null }) {
    super(message);
    this.name = 'UserError';
    this.candidates = opts?.candidates ?? null;
  }
}
