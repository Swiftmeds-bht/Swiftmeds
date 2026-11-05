import Link from "next/link";

export default function NotFound() {
  return (
    <div className="not-found">
      <h1>Page not found</h1>
      <p className="muted">The page you are looking for does not exist or has moved.</p>
      <Link href="/" className="button">
        Go to search
      </Link>
    </div>
  );
}
