"use server";

import { redirect } from "next/navigation";

// The pharmacy portal's form actions (SCRUM-139: moved from the former
// pharmacy-dashboard app). Signing in and out is the app's (app/actions.ts).
import { collectHold, DashboardApiError, setStock, uploadStockCsv } from "@/lib/portal/api";
import { portalSession } from "@/lib/portal/data";
import { collectErrorMessage as collectError } from "@/lib/portal/messages";
import { csvFileProblem } from "@/lib/portal/stock-csv";

function formText(form: FormData, name: string): string {
  const value = form.get(name);
  return typeof value === "string" ? value.trim() : "";
}

function redirectWith(path: string, key: string, value: string): never {
  const query = new URLSearchParams({ [key]: value });
  redirect(`${path}?${query.toString()}`);
}

function actionError(error: unknown, fallback: string): string {
  if (error instanceof DashboardApiError) return error.message;
  return fallback;
}

export async function setStockAction(form: FormData): Promise<void> {
  const productId = formText(form, "productId");
  const rawQuantity = formText(form, "quantityOnHand");

  if (!productId) redirectWith("/pharmacy-portal/stock", "error", "The product ID is missing.");
  if (!/^\d+$/.test(rawQuantity)) {
    redirectWith("/pharmacy-portal/stock", "error", "Enter a whole stock quantity of zero or more.");
  }

  const quantityOnHand = Number(rawQuantity);
  if (!Number.isSafeInteger(quantityOnHand) || quantityOnHand > 2_147_483_647) {
    redirectWith("/pharmacy-portal/stock", "error", "Enter a valid stock quantity.");
  }

  const session = await portalSession();
  if (!session) redirect("/login?next=%2Fpharmacy-portal");

  let message: string | undefined;

  try {
    await setStock(session, productId, quantityOnHand);
  } catch (error) {
    message = actionError(error, "Could not update stock. Please try again.");
  }

  if (message) redirectWith("/pharmacy-portal/stock", "error", message);
  redirectWith("/pharmacy-portal/stock", "notice", "Stock quantity updated.");
}

export async function collectHoldAction(form: FormData): Promise<void> {
  const holdId = formText(form, "holdId");
  const pickupCode = formText(form, "pickupCode");

  if (!holdId) redirectWith("/pharmacy-portal/holds", "error", "The hold ID is missing.");
  if (!pickupCode) redirectWith("/pharmacy-portal/holds", "error", "Enter the pickup code shown by the patient.");

  const session = await portalSession();
  if (!session) redirect("/login?next=%2Fpharmacy-portal");

  let message: string | undefined;

  try {
    await collectHold(session, holdId, pickupCode);
  } catch (error) {
    message = collectError(error);
  }

  if (message) redirectWith("/pharmacy-portal/holds", "error", message);
  redirectWith("/pharmacy-portal/holds", "notice", "Hold collected.");
}
export async function uploadStockCsvAction(form: FormData): Promise<void> {
  const value = form.get("file");
  const file = value instanceof File ? value : null;

  const problem = csvFileProblem(file);
  if (problem || !file) redirectWith("/pharmacy-portal/upload", "error", problem ?? "Choose a CSV file to upload.");

  const session = await portalSession();
  if (!session) redirect("/login?next=%2Fpharmacy-portal");

  let jobId: string | undefined;
  let message: string | undefined;

  try {
    const job = await uploadStockCsv(session, file);
    jobId = job.jobId;
  } catch (error) {
    message = actionError(error, "Could not upload the file. Please try again.");
  }

  if (message || !jobId) redirectWith("/pharmacy-portal/upload", "error", message ?? "Stock Sync returned no job.");
  redirectWith("/pharmacy-portal/upload", "job", jobId);
}
