import type { Metadata } from "next";
import Link from "next/link";
import { redirect } from "next/navigation";

import { PharmacyReviewCard } from "@/components/admin/PharmacyReviewCard";
import { CheckIcon } from "@/components/Icons";
import { getSession } from "@/lib/auth";
import { PENDING, PharmacyApiError, STATUS_TABS, listPharmacies, statusFromSlug, type Pharmacy } from "@/lib/pharmacies";
import { ADMIN_PATH } from "@/lib/roles";

export const metadata: Metadata = { title: "Pharmacies" };
export const dynamic = "force-dynamic";

type SearchParamsIn = Promise<Record<string, string | string[] | undefined>>;
const first = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);

const DONE: Record<string, string> = {
  approved: "was approved. Patients can find it now.",
  rejected: "was rejected. The pharmacy sees the reason when it logs in.",
  suspended: "was suspended and is hidden from patients.",
};

const EMPTY: Record<string, string> = {
  pending: "No pharmacy is waiting for review.",
  verified: "No pharmacy is approved yet.",
  rejected: "No pharmacy was rejected.",
  suspended: "No pharmacy is suspended.",
};

/** The admin's review of pharmacies (SCRUM-142), one tab per status. The layout lets only admins in. */
export default async function AdminPharmaciesPage({ searchParams }: { searchParams: SearchParamsIn }) {
  const session = await getSession();
  if (!session) redirect(`/login?next=${encodeURIComponent(ADMIN_PATH)}`);
  const sp = await searchParams;
  const status = statusFromSlug(first(sp.status));
  const tab = STATUS_TABS.find((t) => t.status === status)!;
  const done = DONE[first(sp.done) ?? ""];
  const doneName = first(sp.name);

  let pharmacies: Pharmacy[] = [];
  let waiting: number | undefined;
  let failed: string | undefined;
  try {
    const [list, pending] = await Promise.all([
      listPharmacies(session, status),
      status === PENDING ? Promise.resolve(undefined) : listPharmacies(session, PENDING).catch(() => undefined),
    ]);
    pharmacies = list;
    waiting = status === PENDING ? list.length : pending?.length;
  } catch (err) {
    if (err instanceof PharmacyApiError && err.status === 401) redirect(`/session-ended?next=${encodeURIComponent(ADMIN_PATH)}`);
    failed = err instanceof PharmacyApiError ? err.message : "SwiftMed could not load the pharmacies.";
  }

  return (
    <div className="page admin-page">
      <div className="admin-head">
        <div>
          <h1>Pharmacies</h1>
          <p className="muted">Check each new pharmacy&apos;s licence and details before patients can find it.</p>
        </div>
        {waiting !== undefined && (
          <p className="admin-waiting">
            <strong>{waiting}</strong> waiting for review
          </p>
        )}
      </div>

      {done && doneName && (
        <p className="notice notice-success" role="status">
          <CheckIcon />
          <span>
            {doneName} {done}
          </span>
        </p>
      )}

      <nav className="admin-tabs" aria-label="Pharmacies by status">
        <ul>
          {STATUS_TABS.map((t) => (
            <li key={t.slug}>
              <Link href={`${ADMIN_PATH}?status=${t.slug}`} aria-current={t.status === status ? "page" : undefined}>
                {t.label}
                {t.status === PENDING && waiting ? <span className="tab-count">{waiting}</span> : null}
              </Link>
            </li>
          ))}
        </ul>
      </nav>

      {failed ? (
        <div className="problem" role="alert">
          <h2>We could not load the pharmacies</h2>
          <p>{failed}</p>
          <Link href={`${ADMIN_PATH}?status=${tab.slug}`} className="button">
            Try again
          </Link>
        </div>
      ) : pharmacies.length === 0 ? (
        <p className="empty">{EMPTY[tab.slug]}</p>
      ) : (
        <div className="review-list">
          {pharmacies.map((p) => (
            <PharmacyReviewCard key={p.pharmacyId} pharmacy={p} tab={tab.slug} />
          ))}
        </div>
      )}
    </div>
  );
}
