import { ResultsSkeleton } from "@/components/ResultsSkeleton";

// Shown while the results page itself loads (for example, right after the first search).
export default function Loading() {
  return (
    <>
      <h1>Results</h1>
      <ResultsSkeleton />
    </>
  );
}
