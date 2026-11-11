// Back-in-stock watchlist for the patient app (SCRUM-71). Like lib/holds.ts:
// with GATEWAY_URL it calls the gateway (docs/api/openapi.yaml, Watchlist), and
// without it an in-memory demo answers. Server-side only: the access token
// never reaches the browser; the browser reads alerts from /api/alerts.
import { demoBackend } from "./demo-holds";
import { errorForStatus } from "./holds";
import { HoldError } from "./holds-core";
import { MOCK_PHARMACIES, MOCK_STOCK } from "./mock-data";
import { parseSSE } from "./sse";
import type { GeoPoint, Session, StockAlert, WatchSubscription } from "./types";

export interface WatchInput {
  productId: string;
  location: GeoPoint;
  radiusM: number;
}

/** How often /api/alerts sends a comment line, so proxies keep a quiet stream open. */
export const ALERT_HEARTBEAT_MS = 15_000;

/** What the browser gets per alert from /api/alerts: the alert plus names to show. */
export interface AlertEvent extends StockAlert {
  medicine: string;
  pharmacy: string;
}

export interface WatchlistBackend {
  readonly source: "gateway" | "demo";
  subscribe(session: Session, input: WatchInput): Promise<WatchSubscription>;
  /** Newest first, including ones that already fired (notifiedAt set). */
  list(session: Session): Promise<WatchSubscription[]>;
  unsubscribe(session: Session, subscriptionId: string): Promise<void>;
  /** Alerts as they happen, until `signal` aborts. Throws HoldError if the stream cannot start or fails. */
  alerts(session: Session, signal: AbortSignal): AsyncGenerator<StockAlert>;
}

export function watchlistBackend(gatewayUrl = process.env.GATEWAY_URL ?? ""): WatchlistBackend {
  return gatewayUrl ? gatewayWatchlist({ gatewayUrl }) : demoWatchlist();
}

// ---------------------------------------------------------------------------
// Gateway

export interface GatewayWatchlistOptions {
  gatewayUrl: string;
  fetchFn?: typeof fetch;
  timeoutMs?: number;
}

export function gatewayWatchlist(options: GatewayWatchlistOptions): WatchlistBackend {
  const fetchFn = options.fetchFn ?? globalThis.fetch;
  const timeoutMs = options.timeoutMs ?? 5000;
  const url = (path: string) => new URL(path, options.gatewayUrl).toString();

  async function send(method: string, path: string, session: Session, body?: unknown, signal?: AbortSignal): Promise<Response> {
    const headers: Record<string, string> = { Authorization: `Bearer ${session.accessToken}` };
    if (body !== undefined) headers["Content-Type"] = "application/json";
    let res: Response;
    try {
      res = await fetchFn(url(path), {
        method,
        headers,
        body: body === undefined ? undefined : JSON.stringify(body),
        cache: "no-store",
        signal: signal ?? AbortSignal.timeout(timeoutMs),
      });
    } catch (cause) {
      const name = (cause as { name?: unknown } | null)?.name;
      const timedOut = name === "TimeoutError";
      throw new HoldError(timedOut ? "timeout" : "unreachable", timedOut ? "The gateway did not answer in time" : "Could not reach the gateway", { cause });
    }
    if (!res.ok) throw errorForStatus(res.status, "hold");
    return res;
  }

  const asSubscription = (body: unknown): WatchSubscription => {
    const s = body as Partial<WatchSubscription> | null;
    if (typeof s?.subscriptionId !== "string" || typeof s.productId !== "string") {
      throw new HoldError("server", "The gateway sent a subscription without an ID");
    }
    return s as WatchSubscription;
  };

  return {
    source: "gateway",

    async subscribe(session, input) {
      const res = await send("POST", "/api/v1/watchlist", session, input);
      return asSubscription(await res.json());
    },

    async list(session) {
      const res = await send("GET", "/api/v1/watchlist", session);
      const body = (await res.json()) as { subscriptions?: unknown[] } | null;
      return (body?.subscriptions ?? []).map(asSubscription);
    },

    async unsubscribe(session, subscriptionId) {
      await send("DELETE", `/api/v1/watchlist/${encodeURIComponent(subscriptionId)}`, session);
    },

    async *alerts(session, signal) {
      const res = await send("GET", "/api/v1/watchlist/alerts", session, undefined, signal);
      if (!res.body) throw new HoldError("server", "The gateway sent no alert stream");
      for await (const event of parseSSE(res.body)) {
        if (event.event === "alert") yield JSON.parse(event.data) as StockAlert;
        else if (event.event === "error") throw new HoldError("server", "The alert stream failed");
      }
    },
  };
}

// ---------------------------------------------------------------------------
// Demo: a watched medicine "comes back" DEMO_RESTOCK_MS after subscribing, at a
// demo pharmacy that stocks it, so the live alert can be seen without a gateway.

export const DEMO_RESTOCK_MS = 10_000;

interface DemoSub extends WatchSubscription {
  userId: string;
  delivered: boolean;
}

export class DemoWatchlist implements WatchlistBackend {
  readonly source = "demo" as const;
  private readonly subs = new Map<string, DemoSub>();

  constructor(
    private readonly now: () => number = Date.now,
    private readonly pollMs = 1000,
  ) {}

  private user(session: Session) {
    return demoBackend().userOf(session);
  }

  async subscribe(session: Session, input: WatchInput): Promise<WatchSubscription> {
    const userId = await this.user(session);
    if (!input.productId) throw new HoldError("bad_request", "Choose a medicine to watch");
    this.advance();
    const existing = [...this.subs.values()].find((s) => s.userId === userId && s.productId === input.productId && !s.notifiedAt);
    if (existing) return view(existing);
    const sub: DemoSub = {
      subscriptionId: crypto.randomUUID(),
      productId: input.productId,
      location: input.location,
      radiusM: input.radiusM,
      createdAt: new Date(this.now()).toISOString(),
      userId,
      delivered: false,
    };
    this.subs.set(sub.subscriptionId, sub);
    return view(sub);
  }

  async list(session: Session): Promise<WatchSubscription[]> {
    const userId = await this.user(session);
    this.advance();
    return [...this.subs.values()]
      .filter((s) => s.userId === userId)
      .sort((a, b) => (b.createdAt ?? "").localeCompare(a.createdAt ?? ""))
      .map(view);
  }

  async unsubscribe(session: Session, subscriptionId: string): Promise<void> {
    const userId = await this.user(session);
    const sub = this.subs.get(subscriptionId);
    if (!sub || sub.userId !== userId) throw new HoldError("not_found", "No such subscription");
    this.subs.delete(subscriptionId);
  }

  async *alerts(session: Session, signal: AbortSignal): AsyncGenerator<StockAlert> {
    const userId = await this.user(session);
    while (!signal.aborted) {
      this.advance();
      for (const sub of this.subs.values()) {
        if (sub.userId === userId && sub.notifiedAt && !sub.delivered) {
          sub.delivered = true;
          yield demoAlert(sub);
        }
      }
      await sleep(this.pollMs, signal);
    }
  }

  /** Subscriptions whose demo restock time has come are notified. */
  private advance() {
    const now = this.now();
    for (const sub of this.subs.values()) {
      const due = Date.parse(sub.createdAt ?? "") + DEMO_RESTOCK_MS;
      if (!sub.notifiedAt && now >= due) sub.notifiedAt = new Date(due).toISOString();
    }
  }
}

/** What the patient sees: the subscription without the demo's bookkeeping. */
function view(sub: DemoSub): WatchSubscription {
  const out: WatchSubscription = { subscriptionId: sub.subscriptionId, productId: sub.productId };
  if (sub.location) out.location = sub.location;
  if (sub.radiusM !== undefined) out.radiusM = sub.radiusM;
  if (sub.createdAt) out.createdAt = sub.createdAt;
  if (sub.notifiedAt) out.notifiedAt = sub.notifiedAt;
  return out;
}

function demoAlert(sub: DemoSub): StockAlert {
  const stock = MOCK_STOCK.find((s) => s.productId === sub.productId && s.quantity > 0);
  return {
    subscriptionId: sub.subscriptionId,
    productId: sub.productId,
    pharmacyId: stock?.pharmacyId ?? MOCK_PHARMACIES[0].pharmacyId,
    quantityAvailable: stock?.quantity ?? 3,
    occurredAt: sub.notifiedAt ?? new Date().toISOString(),
  };
}

function sleep(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    const timer = setTimeout(resolve, ms);
    signal.addEventListener("abort", () => (clearTimeout(timer), resolve()), { once: true });
  });
}

const globalStore = globalThis as typeof globalThis & { __swiftmedDemoWatchlist?: DemoWatchlist };

export function demoWatchlist(): DemoWatchlist {
  globalStore.__swiftmedDemoWatchlist ??= new DemoWatchlist();
  return globalStore.__swiftmedDemoWatchlist;
}
