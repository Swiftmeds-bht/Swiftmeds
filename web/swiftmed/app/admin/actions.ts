"use server";

// The administrator's decisions on a pharmacy (SCRUM-142): approve, reject or
// suspend, through setPharmacyVerification (SCRUM-141).
import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";

import { clearSession, getSession, withFreshToken } from "@/lib/auth";
import { PharmacyApiError, REJECTED, SUSPENDED, VERIFIED, setVerification, type VerificationStatus } from "@/lib/pharmacies";
import { ADMIN_PATH, isAdmin } from "@/lib/roles";

export interface ReviewState {
  error?: string;
}

const DECISIONS: Record<string, { status: VerificationStatus; needsReason: boolean; done: string }> = {
  approve: { status: VERIFIED, needsReason: false, done: "approved" },
  reject: { status: REJECTED, needsReason: true, done: "rejected" },
  suspend: { status: SUSPENDED, needsReason: true, done: "suspended" },
};

const text = (form: FormData, name: string) => {
  const value = form.get(name);
  return typeof value === "string" ? value.trim() : "";
};

export async function reviewPharmacyAction(_prev: ReviewState, form: FormData): Promise<ReviewState> {
  const session = await getSession();
  if (!session) redirect(`/login?next=${encodeURIComponent(ADMIN_PATH)}`);
  if (!isAdmin(session)) return { error: "Only SwiftMed administrators can do this." };

  const decision = DECISIONS[text(form, "decision")];
  const pharmacyId = text(form, "pharmacyId");
  const name = text(form, "name") || "The pharmacy";
  const tab = text(form, "tab") || "pending";
  const reason = text(form, "reason");
  if (!decision || !pharmacyId) return { error: "Something is missing from the form. Reload the page and try again." };
  if (decision.needsReason && !reason) return { error: "Write the reason. The pharmacy sees it." };
  if (reason.length > 500) return { error: "Keep the reason under 500 characters." };

  try {
    await withFreshToken(session, (s) => setVerification(s, pharmacyId, decision.status, decision.needsReason ? reason : ""));
  } catch (err) {
    if (err instanceof PharmacyApiError && err.status === 401) {
      await clearSession();
      redirect(`/login?next=${encodeURIComponent(ADMIN_PATH)}&expired=1`);
    }
    if (err instanceof PharmacyApiError) return { error: err.message };
    console.error(err);
    return { error: "SwiftMed could not save that just now. Please try again." };
  }
  revalidatePath(ADMIN_PATH);
  redirect(`${ADMIN_PATH}?status=${encodeURIComponent(tab)}&done=${decision.done}&name=${encodeURIComponent(name)}`);
}
