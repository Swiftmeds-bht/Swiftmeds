// Shown while the pharmacies load. The blocks match the shape of a result and are
// decoration; the message is what screen readers announce.
export function ResultsSkeleton() {
  return (
    <div className="skeleton" role="status">
      <p>Searching nearby pharmacies…</p>
      {[1, 2].map((n) => (
        <div key={n} className="skeleton-card" aria-hidden="true">
          <div className="skeleton-head">
            <div className="skeleton-line skeleton-title" />
            <div className="skeleton-line skeleton-short" />
          </div>
          {[1, 2].map((row) => (
            <div key={row} className="skeleton-row">
              <div className="skeleton-line" />
              <div className="skeleton-line skeleton-side" />
              <div className="skeleton-line skeleton-short" />
            </div>
          ))}
        </div>
      ))}
    </div>
  );
}
