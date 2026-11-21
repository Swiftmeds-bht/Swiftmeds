"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";

import { formatClock, formatRemaining } from "@/lib/hold-format";

interface Props {
  expiresAt: string;
  /** For tests. */
  now?: () => number;
}

// Time left on an active hold. It ticks every second but is not announced each
// second: screen readers get the fixed "until 14:32" text instead. When the time
// is up it reloads the page, so the hold shows as expired.
export function Countdown({ expiresAt, now = Date.now }: Props) {
  const router = useRouter();
  const end = Date.parse(expiresAt);
  const [left, setLeft] = useState(() => end - now());

  useEffect(() => {
    const timer = setInterval(() => {
      const remaining = end - now();
      setLeft(remaining);
      if (remaining <= 0) {
        clearInterval(timer);
        router.refresh();
      }
    }, 1000);
    return () => clearInterval(timer);
  }, [end, now, router]);

  return (
    <p className="countdown">
      {/* The server and the browser render a second apart; the next tick corrects it. */}
      <span className="countdown-time" aria-hidden="true" suppressHydrationWarning>
        {formatRemaining(left)}
      </span>
      <span className="countdown-label">
        left <span className="sr-only">to collect, </span>until {formatClock(expiresAt)}
      </span>
    </p>
  );
}
