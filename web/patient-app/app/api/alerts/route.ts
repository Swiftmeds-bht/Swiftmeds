import { getSession } from "@/lib/auth";
import { holdNames } from "@/lib/hold-format";
import { HoldError } from "@/lib/holds";
import { formatSSE } from "@/lib/sse";
import { ALERT_HEARTBEAT_MS, watchlistBackend, type AlertEvent } from "@/lib/watchlist";

// The browser's EventSource reads back-in-stock alerts here (SCRUM-71). The
// route reads the session cookie and asks the gateway with the access token,
// so the token never reaches page scripts (EventSource cannot send headers).
export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  const session = await getSession();
  if (!session) return new Response("Not signed in", { status: 401, headers: { "Cache-Control": "no-store" } });

  const backend = watchlistBackend();
  const encoder = new TextEncoder();
  const upstream = new AbortController();
  const stop = () => upstream.abort();
  request.signal.addEventListener("abort", stop, { once: true });

  const body = new ReadableStream<Uint8Array>({
    async start(controller) {
      const write = (chunk: string) => {
        try {
          controller.enqueue(encoder.encode(chunk));
        } catch {
          stop(); // the browser is gone
        }
      };
      const heartbeat = setInterval(() => write(": ping\n\n"), ALERT_HEARTBEAT_MS);
      write(": connected\n\n");
      try {
        for await (const alert of backend.alerts(session, upstream.signal)) {
          const names = holdNames(alert);
          const event: AlertEvent = { ...alert, medicine: names.medicine, pharmacy: names.pharmacy };
          write(formatSSE("alert", event, alert.subscriptionId));
        }
      } catch (err) {
        if (!upstream.signal.aborted) {
          // "unauthorized" tells the page to stop retrying; anything else it retries.
          const kind = err instanceof HoldError ? err.kind : "server";
          if (!(err instanceof HoldError)) console.error("alert stream failed:", err);
          write(formatSSE("error", { kind }));
        }
      } finally {
        clearInterval(heartbeat);
        request.signal.removeEventListener("abort", stop);
        try {
          controller.close();
        } catch {
          // already closed by the browser leaving
        }
      }
    },
    cancel: stop,
  });

  return new Response(body, {
    headers: {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-cache, no-transform",
      Connection: "keep-alive",
      "X-Accel-Buffering": "no",
    },
  });
}
