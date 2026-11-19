"use server";

import { redirect } from "next/navigation";

import {
  collectHold,
  DashboardApiError,
  login,
  setStock,
  uploadStockCsv,
} from "@/lib/api";
import { collectErrorMessage as collectError, loginErrorMessage as loginError } from "@/lib/messages";
import { csvFileProblem } from "@/lib/stock-csv";
import {
  clearSession,
  getSession,
  saveSession,
  sessionFromLogin,
} from "@/lib/session";

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

export async function loginAction(form: FormData): Promise<void> {
  const email = formText(form, "email");
  const password = form.get("password");

  if (!email || typeof password !== "string" || !password) {
    redirectWith("/login", "error", "Enter your email and password.");
  }

  let message: string | undefined;

  try {
    const response = await login(email, password);
    const session = sessionFromLogin(response, email);
    await saveSession(session);
  } catch (error) {
    message = loginError(error);
  }

  if (message) redirectWith("/login", "error", message);
  redirect("/");
}

export async function logoutAction(): Promise<void> {
  await clearSession();
  redirectWith("/login", "notice", "You have been signed out.");
}

export async function setStockAction(form: FormData): Promise<void> {
  const productId = formText(form, "productId");
  const rawQuantity = formText(form, "quantityOnHand");

  if (!productId) redirectWith("/stock", "error", "The product ID is missing.");
  if (!/^\d+$/.test(rawQuantity)) {
    redirectWith("/stock", "error", "Enter a whole stock quantity of zero or more.");
  }

  const quantityOnHand = Number(rawQuantity);
  if (!Number.isSafeInteger(quantityOnHand) || quantityOnHand > 2_147_483_647) {
    redirectWith("/stock", "error", "Enter a valid stock quantity.");
  }

  const session = await getSession();
  if (!session) redirect("/login");

  let message: string | undefined;

  try {
    await setStock(session, productId, quantityOnHand);
  } catch (error) {
    message = actionError(error, "Could not update stock. Please try again.");
  }

  if (message) redirectWith("/stock", "error", message);
  redirectWith("/stock", "notice", "Stock quantity updated.");
}

export async function collectHoldAction(form: FormData): Promise<void> {
  const holdId = formText(form, "holdId");
  const pickupCode = formText(form, "pickupCode");

  if (!holdId) redirectWith("/holds", "error", "The hold ID is missing.");
  if (!pickupCode) redirectWith("/holds", "error", "Enter the pickup code shown by the patient.");

  const session = await getSession();
  if (!session) redirect("/login");

  let message: string | undefined;

  try {
    await collectHold(session, holdId, pickupCode);
  } catch (error) {
    message = collectError(error);
  }

  if (message) redirectWith("/holds", "error", message);
  redirectWith("/holds", "notice", "Hold collected.");
}
export async function uploadStockCsvAction(form: FormData): Promise<void> {
  const value = form.get("file");
  const file = value instanceof File ? value : null;

  const problem = csvFileProblem(file);
  if (problem || !file) redirectWith("/upload", "error", problem ?? "Choose a CSV file to upload.");

  const session = await getSession();
  if (!session) redirect("/login");

  let jobId: string | undefined;
  let message: string | undefined;

  try {
    const job = await uploadStockCsv(session, file);
    jobId = job.jobId;
  } catch (error) {
    message = actionError(error, "Could not upload the file. Please try again.");
  }

  if (message || !jobId) redirectWith("/upload", "error", message ?? "Stock Sync returned no job.");
  redirectWith("/upload", "job", jobId);
}
