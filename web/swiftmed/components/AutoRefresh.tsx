"use client";

import { useRouter } from "next/navigation";
import { useEffect } from "react";

interface Props {
  /** How often to reload the server data. */
  everyMs?: number;
  /** Stop after this long, so a stuck hold does not poll forever. */
  forMs?: number;
}

// Reloads the page's server data while a hold is pending (the API is poll-based:
// openapi.yaml, POST /api/v1/holds). Renders nothing.
export function AutoRefresh({ everyMs = 2000, forMs = 2 * 60_000 }: Props) {
  const router = useRouter();
  useEffect(() => {
    const started = Date.now();
    const timer = setInterval(() => {
      if (Date.now() - started > forMs) {
        clearInterval(timer);
        return;
      }
      router.refresh();
    }, everyMs);
    return () => clearInterval(timer);
  }, [everyMs, forMs, router]);
  return null;
}
