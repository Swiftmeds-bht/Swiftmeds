// In-memory demo of accounts and holds, used when no GATEWAY_URL is set. It follows
// the Holds rules in docs/api/openapi.yaml so the screens can be built and tested
// before the gateway routes exist. Data lives in this server process only and is
// lost on restart. Not for real patients.
//
// Demo rules:
//   - A hold is PENDING for PENDING_MS, then ACTIVE for HOLD_MINUTES, then EXPIRED.
//   - It is REJECTED instead if more units are asked for than are left (stock minus
//     other active holds), or if the pharmacy had only its last unit: the demo
//     pretends someone else reserved it first, so the rejected screen can be seen.
import { createHash, randomBytes, randomInt, randomUUID } from "node:crypto";

import { HoldError, MAX_HOLD_QUANTITY, type HoldsBackend } from "./holds-core";
import { MOCK_STOCK } from "./mock-data";
import type { CreateHoldInput, Hold, Session } from "./types";

export const PENDING_MS = 2000;
export const HOLD_MINUTES = 30;
export const DEMO_ACCOUNT = { email: "patient@demo.bt", password: "swiftmed-demo" } as const;

const MIN_PASSWORD = 8;

interface StoredHold extends Hold {
  userId: string;
  pharmacyId: string;
  productId: string;
  quantity: number;
  createdAt: string;
}

const holdError = (kind: HoldError["kind"], message: string) => new HoldError(kind, message);

const hash = (salt: string, password: string) => createHash("sha256").update(`${salt}:${password}`).digest("hex");

export class DemoHolds implements HoldsBackend {
  readonly source = "demo" as const;
  private readonly users = new Map<string, { userId: string; salt: string; hash: string }>();
  private readonly tokens = new Map<string, string>(); // access token -> userId
  private readonly holds = new Map<string, StoredHold>();

  constructor(private readonly now: () => number = Date.now) {
    const salt = "demo";
    this.users.set(DEMO_ACCOUNT.email, { userId: randomUUID(), salt, hash: hash(salt, DEMO_ACCOUNT.password) });
  }

  async register(email: string, password: string): Promise<void> {
    const key = email.trim().toLowerCase();
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(key)) throw holdError("bad_request", "Enter a valid email address");
    if (password.length < MIN_PASSWORD) throw holdError("bad_request", `Use at least ${MIN_PASSWORD} characters`);
    if (this.users.has(key)) throw holdError("email_taken", "The email is already registered");
    const salt = randomBytes(8).toString("hex");
    this.users.set(key, { userId: randomUUID(), salt, hash: hash(salt, password) });
  }

  async login(email: string, password: string): Promise<Session> {
    const key = email.trim().toLowerCase();
    const user = this.users.get(key);
    if (!user || user.hash !== hash(user.salt, password)) {
      throw holdError("invalid_credentials", "Wrong email or password");
    }
    const accessToken = randomBytes(24).toString("hex");
    this.tokens.set(accessToken, user.userId);
    return { userId: user.userId, email: key, accessToken };
  }

  async createHold(session: Session, input: CreateHoldInput): Promise<Hold> {
    const userId = await this.user(session);
    if (!Number.isInteger(input.quantity) || input.quantity < 1 || input.quantity > MAX_HOLD_QUANTITY) {
      throw holdError("bad_request", `Choose 1 to ${MAX_HOLD_QUANTITY} units`);
    }
    if (!this.stock(input.pharmacyId, input.productId)) {
      throw holdError("not_found", "This pharmacy does not stock that medicine");
    }
    const at = new Date(this.now()).toISOString();
    const hold: StoredHold = {
      holdId: randomUUID(),
      userId,
      pharmacyId: input.pharmacyId,
      productId: input.productId,
      quantity: input.quantity,
      status: "HOLD_STATUS_PENDING",
      createdAt: at,
      statusChangedAt: at,
    };
    this.holds.set(hold.holdId, hold);
    return this.view(hold);
  }

  async getHold(session: Session, holdId: string): Promise<Hold | undefined> {
    const userId = await this.user(session);
    const hold = this.holds.get(holdId);
    if (!hold || hold.userId !== userId) return undefined;
    this.advance();
    return this.view(hold);
  }

  async listHolds(session: Session): Promise<Hold[]> {
    const userId = await this.user(session);
    this.advance();
    return [...this.holds.values()]
      .filter((h) => h.userId === userId)
      .sort((a, b) => b.createdAt.localeCompare(a.createdAt) || a.holdId.localeCompare(b.holdId))
      .map((h) => this.view(h));
  }

  async cancelHold(session: Session, holdId: string): Promise<Hold> {
    const userId = await this.user(session);
    const hold = this.holds.get(holdId);
    if (!hold || hold.userId !== userId) throw holdError("not_found", "No such hold");
    this.advance();
    if (hold.status !== "HOLD_STATUS_ACTIVE") throw holdError("not_active", "The hold is no longer active");
    this.set(hold, "HOLD_STATUS_CANCELLED", this.now(), "Cancelled by the patient");
    return this.view(hold);
  }

  /** The signed-in demo user's ID; also used by the demo watchlist. */
  async userOf(session: Session): Promise<string> {
    return this.user(session);
  }

  private async user(session: Session): Promise<string> {
    const userId = this.tokens.get(session.accessToken);
    if (!userId || userId !== session.userId) throw holdError("unauthorized", "Please log in again");
    return userId;
  }

  private stock(pharmacyId: string, productId: string) {
    return MOCK_STOCK.find((s) => s.pharmacyId === pharmacyId && s.productId === productId && s.quantity > 0);
  }

  /** Moves every hold on to where it should be now, oldest first, so stock is shared fairly. */
  private advance() {
    const now = this.now();
    const ordered = [...this.holds.values()].sort((a, b) => a.createdAt.localeCompare(b.createdAt));
    for (const hold of ordered) {
      const created = Date.parse(hold.createdAt);
      if (hold.status === "HOLD_STATUS_PENDING" && now >= created + PENDING_MS) {
        this.resolve(hold, created + PENDING_MS);
      }
      if (hold.status === "HOLD_STATUS_ACTIVE" && hold.expiresAt && now >= Date.parse(hold.expiresAt)) {
        this.set(hold, "HOLD_STATUS_EXPIRED", Date.parse(hold.expiresAt));
      }
    }
  }

  private resolve(hold: StoredHold, at: number) {
    const stock = this.stock(hold.pharmacyId, hold.productId);
    const held = [...this.holds.values()]
      .filter((h) => h !== hold && h.status === "HOLD_STATUS_ACTIVE" && h.pharmacyId === hold.pharmacyId && h.productId === hold.productId)
      .reduce((n, h) => n + h.quantity, 0);
    const left = (stock?.quantity ?? 0) - held;

    if (stock?.quantity === 1) {
      this.set(hold, "HOLD_STATUS_REJECTED", at, "Someone else reserved the last unit just before you.");
    } else if (hold.quantity > left) {
      this.set(hold, "HOLD_STATUS_REJECTED", at, left > 0 ? `Only ${left} left at this pharmacy.` : "This pharmacy has none left.");
    } else {
      hold.pickupCode = String(randomInt(0, 1_000_000)).padStart(6, "0");
      hold.expiresAt = new Date(at + HOLD_MINUTES * 60_000).toISOString();
      this.set(hold, "HOLD_STATUS_ACTIVE", at);
    }
  }

  private set(hold: StoredHold, status: Hold["status"], at: number, reason?: string) {
    hold.status = status;
    hold.statusChangedAt = new Date(at).toISOString();
    if (reason) hold.statusReason = reason;
  }

  /** A copy, with the pickup code only while the hold can be collected. */
  private view(hold: StoredHold): Hold {
    const copy: Hold = { ...hold };
    if (hold.status !== "HOLD_STATUS_ACTIVE") delete copy.pickupCode;
    return copy;
  }
}

// One demo store per server process. Kept on globalThis so that `next dev`
// reloading this module does not sign everyone out.
const globalStore = globalThis as typeof globalThis & { __swiftmedDemoHolds?: DemoHolds };

export function demoBackend(): DemoHolds {
  globalStore.__swiftmedDemoHolds ??= new DemoHolds();
  return globalStore.__swiftmedDemoHolds;
}
