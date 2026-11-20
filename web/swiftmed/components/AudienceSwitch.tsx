import Link from "next/link";

interface Props {
  /** What the switch is for, read by screen readers ("Log in as", "Sign up as"). */
  label: string;
  options: { href: string; label: string; current: boolean }[];
}

/**
 * Two links that look like a segmented control (SCRUM-140, SCRUM-142): the
 * patient and the pharmacy version of the same page.
 */
export function AudienceSwitch({ label, options }: Props) {
  return (
    <nav className="login-switch" aria-label={label}>
      {options.map((o) => (
        <Link key={o.href} href={o.href} aria-current={o.current ? "page" : undefined}>
          {o.label}
        </Link>
      ))}
    </nav>
  );
}
