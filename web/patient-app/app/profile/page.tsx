import type { Metadata } from "next";
import Link from "next/link";
import { redirect } from "next/navigation";

import { ProfileForm } from "@/components/ProfileForm";
import { requireSession } from "@/lib/auth";
import { errorMessage } from "@/lib/hold-format";
import { HoldError } from "@/lib/holds";
import { profileBackend, type Profile } from "@/lib/profile";

export const metadata: Metadata = { title: "Your profile" };
export const dynamic = "force-dynamic";

/** The signed-in patient's name and phone (SCRUM-137). */
export default async function ProfilePage() {
  const session = await requireSession("/profile");

  let profile: Profile | undefined;
  let failed: HoldError["kind"] | undefined;
  try {
    profile = await profileBackend().get(session);
  } catch (err) {
    failed = err instanceof HoldError ? err.kind : "server";
  }
  if (failed === "unauthorized") redirect("/session-ended?next=%2Fprofile");

  return (
    <div className="page narrow profile-page">
      <h1>Your profile</h1>
      <p className="muted">
        Signed in as <strong>{session.email}</strong>.
      </p>

      {failed ? (
        <div className="problem" role="alert">
          <h2>We could not load your profile</h2>
          <p>{errorMessage(failed)}</p>
          <Link href="/profile" className="button">
            Try again
          </Link>
        </div>
      ) : (
        <ProfileForm displayName={profile?.displayName ?? ""} phone={profile?.phone ?? ""} />
      )}
    </div>
  );
}
