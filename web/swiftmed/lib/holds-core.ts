// What every holds backend (gateway or demo) shares: the contract and its errors.
import type { CreateHoldInput, Hold, Session } from "./types";

export type HoldErrorKind =
  | "invalid_credentials" // wrong email or password
  | "email_taken"
  | "unauthorized" // not signed in, or the session ended
  | "not_found"
  | "not_active" // the hold is no longer active (expired, collected, cancelled)
  | "bad_request"
  | "rate_limited"
  | "not_available" // the gateway has no such route yet
  | "unreachable"
  | "timeout"
  | "server";

/** Why an account or hold call failed, in terms a screen can explain. */
export class HoldError extends Error {
  readonly kind: HoldErrorKind;

  constructor(kind: HoldErrorKind, message: string, options?: ErrorOptions) {
    super(message, options);
    this.name = "HoldError";
    this.kind = kind;
  }
}

/** What the screens need. The gateway and the demo both implement it. */
export interface HoldsBackend {
  readonly source: "gateway" | "demo";
  register(email: string, password: string): Promise<void>;
  login(email: string, password: string): Promise<Session>;
  createHold(session: Session, input: CreateHoldInput): Promise<Hold>;
  /** Returns undefined when the hold does not exist or is not the user's. */
  getHold(session: Session, holdId: string): Promise<Hold | undefined>;
  /** The user's holds, newest first. */
  listHolds(session: Session): Promise<Hold[]>;
  cancelHold(session: Session, holdId: string): Promise<Hold>;
}

/** A hold is for 1 to 5 units (openapi.yaml CreateHoldRequest). */
export const MAX_HOLD_QUANTITY = 5;
