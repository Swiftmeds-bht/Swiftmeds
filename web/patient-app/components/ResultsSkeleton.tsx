// Shown while the pharmacies load. The grey blocks are decoration; the
// message is what screen readers announce.
export function ResultsSkeleton() {
  return (
    <div className="skeleton" role="status">
      <p>Searching nearby pharmacies…</p>
      {[1, 2].map((n) => (
        <div key={n} className="skeleton-card" aria-hidden="true">
          <div className="skeleton-line skeleton-title" />
          <div className="skeleton-line" />
          <div className="skeleton-line skeleton-short" />
        </div>
      ))}
    </div>
  );
}
