// Notifications for the header bell (SCRUM-137): GET /api/v1/notifications
// through the gateway (docs/api/openapi.yaml, listNotifications). The Notification
// service stores one per hold confirmed, expired or rejected, and per
// back-in-stock alert. There is no "mark as read" API, so the app remembers the
// newest notification the patient has seen in a cookie and counts newer ones as
// unread. Server-side only: the access token never reaches the browser.
import { gatewayUrl as configuredGatewayUrl } from "./data-source";
import { holdNames } from "./hold-format";
import { errorForStatus } from "./holds";
import { HoldError } from "./holds-core";
import { EMPTY_BOOK, type NameBook } from "./name-book";
import type { Session } from "./types";

/** Cookie with the creation time of the newest notification the patient has seen. */
export const NOTIFICATIONS_SEEN_COOKIE = "swiftmed_notifications_seen";

/** How many notifications the bell shows. */
export const BELL_LIMIT = 8;

/** One notification as the gateway sends it. */
export interface AppNotification {
  notificationId: string;
  template: string;
  params: Record<string, string>;
  createdAt?: string;
}

/** What the bell shows for one notification. */
export interface NotificationView {
  id: string;
  title: string;
  body: string;
  href?: string;
  createdAt?: string;
}

const holdLink = (params: Record<string, string>) =>
  params.hold_id ? `/holds/${encodeURIComponent(params.hold_id)}` : "/holds";

/** Plain words for a notification, from its template and values; `book` names the medicine and pharmacy. */
export function describeNotification(n: AppNotification, book: NameBook = EMPTY_BOOK): NotificationView {
  const p = n.params;
  const base = { id: n.notificationId, createdAt: n.createdAt };
  switch (n.template) {
    case "TEMPLATE_HOLD_CONFIRMED":
      return {
        ...base,
        title: "Hold confirmed",
        body: p.pickup_code ? `Show pickup code ${p.pickup_code} at the counter.` : "Your medicine is set aside for you.",
        href: holdLink(p),
      };
    case "TEMPLATE_HOLD_EXPIRED":
      return { ...base, title: "Hold expired", body: "The time to collect ran out, so the units were released.", href: holdLink(p) };
    case "TEMPLATE_HOLD_REJECTED":
      return { ...base, title: "Hold not placed", body: "The pharmacy could not set the medicine aside. Try another pharmacy.", href: holdLink(p) };
    case "TEMPLATE_BACK_IN_STOCK": {
      const { medicine, pharmacy } = holdNames({ productId: p.product_id ?? "", pharmacyId: p.pharmacy_id ?? "" }, book);
      return {
        ...base,
        title: `${medicine} is back in stock`,
        body: `Available at ${pharmacy}.`,
        href: p.pharmacy_id ? `/pharmacy/${encodeURIComponent(p.pharmacy_id)}` : "/watchlist",
      };
    }
    default:
      return { ...base, title: "Update from SwiftMed", body: "Open My holds for the latest.", href: "/holds" };
  }
}

const time = (iso?: string) => {
  const ms = iso ? Date.parse(iso) : Number.NaN;
  return Number.isNaN(ms) ? undefined : ms;
};

/** How many notifications are newer than `seenAt` (all of them when nothing was seen yet). */
export function unreadCount(list: Pick<AppNotification, "createdAt">[], seenAt?: string): number {
  const seen = time(seenAt);
  if (seen === undefined) return list.length;
  return list.filter((n) => (time(n.createdAt) ?? 0) > seen).length;
}

/** The newest creation time in the list, to remember as seen. */
export function newestCreatedAt(list: Pick<AppNotification, "createdAt">[]): string | undefined {
  let newest: { ms: number; iso: string } | undefined;
  for (const n of list) {
    const ms = time(n.createdAt);
    if (ms !== undefined && (!newest || ms > newest.ms)) newest = { ms, iso: n.createdAt! };
  }
  return newest?.iso;
}

/** A value from the seen cookie, or undefined if it is not a date. */
export function parseSeenAt(value: string | undefined): string | undefined {
  return time(value) === undefined ? undefined : value;
}

export interface NotificationsBackend {
  readonly source: "gateway" | "demo";
  /** Newest first, at most `limit`. */
  list(session: Session, limit?: number): Promise<AppNotification[]>;
}

export function notificationsBackend(gatewayUrl = configuredGatewayUrl()): NotificationsBackend {
  return gatewayUrl ? gatewayNotifications({ gatewayUrl }) : { source: "demo", list: async () => [] };
}

export interface GatewayNotificationsOptions {
  gatewayUrl: string;
  fetchFn?: typeof fetch;
  timeoutMs?: number;
}

const asNotification = (raw: unknown): AppNotification | undefined => {
  const n = raw as Partial<AppNotification> | null;
  if (typeof n?.notificationId !== "string" || !n.notificationId) return undefined;
  const params: Record<string, string> = {};
  for (const [key, value] of Object.entries(n.params ?? {})) if (typeof value === "string") params[key] = value;
  return {
    notificationId: n.notificationId,
    template: typeof n.template === "string" ? n.template : "TEMPLATE_UNSPECIFIED",
    params,
    createdAt: typeof n.createdAt === "string" ? n.createdAt : undefined,
  };
};

export function gatewayNotifications(options: GatewayNotificationsOptions): NotificationsBackend {
  const fetchFn = options.fetchFn ?? globalThis.fetch;
  const timeoutMs = options.timeoutMs ?? 3000;
  return {
    source: "gateway",
    async list(session, limit = BELL_LIMIT) {
      const url = new URL(`/api/v1/notifications?pageSize=${limit}`, options.gatewayUrl);
      let res: Response;
      try {
        res = await fetchFn(url, {
          headers: { Authorization: `Bearer ${session.accessToken}` },
          cache: "no-store",
          signal: AbortSignal.timeout(timeoutMs),
        });
      } catch (cause) {
        const timedOut = (cause as { name?: unknown } | null)?.name === "TimeoutError";
        throw new HoldError(timedOut ? "timeout" : "unreachable", "Could not load notifications", { cause });
      }
      if (!res.ok) throw errorForStatus(res.status, "hold");
      const body = (await res.json().catch(() => null)) as { notifications?: unknown[] } | null;
      const list = (body?.notifications ?? []).map(asNotification).filter((n): n is AppNotification => n !== undefined);
      // Newest first, whatever order the service used.
      return list.sort((a, b) => (time(b.createdAt) ?? 0) - (time(a.createdAt) ?? 0)).slice(0, limit);
    },
  };
}
