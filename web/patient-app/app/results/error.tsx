"use client";

import { useEffect } from "react";

import { SearchProblem } from "@/components/SearchProblem";

// The last safety net: anything unexpected on the results page lands here.
export default function ResultsError({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  useEffect(() => {
    console.error(error);
  }, [error]);

  return (
    <>
      <h1>Results</h1>
      <SearchProblem kind="unexpected" onRetry={reset} />
    </>
  );
}
