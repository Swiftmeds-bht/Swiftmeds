import Link from "next/link";

import { CompassIcon } from "@/components/Icons";

export default function NotFound() {
  return (
    <div className="page not-found">
      <span className="not-found-icon">
        <CompassIcon size={28} />
      </span>
      <h1>Page not found</h1>
      <p className="muted">The page you are looking for does not exist or has moved.</p>
      <Link href="/" className="button">
        Go to search
      </Link>
    </div>
  );
}
