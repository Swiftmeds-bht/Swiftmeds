"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";

import { formatQuantity } from "@/lib/format";
import type { AlertEvent } from "@/lib/watchlist";

/** Longest wait between reconnects after the stream fails. */
const MAX_RETRY_MS = 60_000;

// Listens for back-in-stock alerts while a patient is signed in, and shows each
// one as it arrives, without a page refresh (SCRUM-71). Rendered by the layout.
export function AlertListener({ url = "/api/alerts" }: { url?: string }) {
  const [alerts, setAlerts] = useState<AlertEvent[]>([]);
  const router = useRouter();

  useEffect(() => {
    if (typeof EventSource === "undefined") return;
    let source: EventSource | undefined;
    let retry: ReturnType<typeof setTimeout> | undefined;
    let delay = 2000;
    let stopped = false;

    const connect = () => {
      source = new EventSource(url);
      source.addEventListener("open", () => (delay = 2000));
      source.addEventListener("alert", (e) => {
        const alert = JSON.parse((e as MessageEvent<string>).data) as AlertEvent;
        setAlerts((list) => (list.some((a) => a.subscriptionId === alert.subscriptionId) ? list : [...list, alert]));
        // Server-rendered parts (e.g. the watchlist's "Waiting") catch up, keeping scroll and input.
        router.refresh();
      });
      // Our own "error" event carries data; the browser's connection error does not.
      source.addEventListener("error", (e) => {
        const data = (e as MessageEvent<string>).data;
        const kind = data ? ((JSON.parse(data) as { kind?: string }).kind ?? "server") : "connection";
        source?.close();
        if (stopped || kind === "unauthorized") return; // signed out: stop asking
        retry = setTimeout(connect, delay);
        delay = Math.min(delay * 2, MAX_RETRY_MS);
      });
    };
    connect();

    return () => {
      stopped = true;
      clearTimeout(retry);
      source?.close();
    };
  }, [url, router]);

  if (alerts.length === 0) return null;
  const dismiss = (id: string) => setAlerts((list) => list.filter((a) => a.subscriptionId !== id));

  return (
    <div className="alert-stack" role="status" aria-live="polite">
      {alerts.map((a) => (
        <div key={a.subscriptionId} className="alert-toast">
          <p className="alert-title">{a.medicine} is back in stock</p>
          <p className="alert-body">
            {formatQuantity(a.quantityAvailable)} at {a.pharmacy}.
          </p>
          <div className="alert-actions">
            <Link href={`/pharmacy/${encodeURIComponent(a.pharmacyId)}`} className="button button-small" onClick={() => dismiss(a.subscriptionId)}>
              See pharmacy
            </Link>
            <button type="button" className="link-button" onClick={() => dismiss(a.subscriptionId)}>
              Dismiss<span className="sr-only"> alert for {a.medicine}</span>
            </button>
          </div>
        </div>
      ))}
    </div>
  );
}
