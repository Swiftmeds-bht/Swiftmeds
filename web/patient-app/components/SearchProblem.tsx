import Link from "next/link";

import type { SearchErrorKind } from "@/lib/api";

export type ProblemKind = SearchErrorKind | "unexpected";

const MESSAGES: Record<ProblemKind, { title: string; body: string }> = {
  unreachable: {
    title: "We could not reach the pharmacy search",
    body: "Check your connection and try again in a moment.",
  },
  timeout: {
    title: "The search is taking too long",
    body: "Pharmacy results did not arrive in time. Try again in a moment.",
  },
  server: {
    title: "The search is not working right now",
    body: "Something went wrong on our side. Try again in a moment.",
  },
  not_available: {
    title: "Live search is not available yet",
    body: "The pharmacy search is still being connected. Please try again later.",
  },
  bad_request: {
    title: "That search was not accepted",
    body: "Type a medicine name of 2 to 100 characters and search again.",
  },
  unexpected: {
    title: "Something went wrong",
    body: "We could not show the results. Try again, or start a new search.",
  },
};

interface Props {
  kind: ProblemKind;
  /** A link back to the same search (used on the server). */
  retryHref?: string;
  /** Called by the Try again button (used by the error boundary). */
  onRetry?: () => void;
}

export function SearchProblem({ kind, retryHref, onRetry }: Props) {
  const message = MESSAGES[kind];
  return (
    <div className="problem" role="alert">
      <h2>{message.title}</h2>
      <p>{message.body}</p>
      {onRetry ? (
        <button type="button" className="button" onClick={onRetry}>
          Try again
        </button>
      ) : retryHref && kind !== "bad_request" ? (
        <a className="button" href={retryHref}>
          Try again
        </a>
      ) : null}
      <p className="hint">
        You can also <Link href="/">start a new search</Link>.
      </p>
    </div>
  );
}
