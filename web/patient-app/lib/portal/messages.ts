// User-facing messages for failed dashboard actions. Kept out of app/actions.ts
// because a "use server" file may only export async functions, and these need
// unit tests.
import { DashboardApiError } from "./api";

/**
 * Explains why collecting a hold failed. The Holds service answers a wrong
 * pickup code (or a hold from another pharmacy) with 403 and leaves the hold
 * active; 412 means the hold is no longer active.
 */
export function collectErrorMessage(error: unknown): string {
  if (!(error instanceof DashboardApiError)) {
    return "Could not collect this hold. Please try again.";
  }

  switch (error.status) {
    case 400:
    case 403:
      return "Wrong pickup code. Check the code with the patient and try again. The hold is still active.";
    case 404:
      return "This hold was not found. Refresh the dashboard.";
    case 409:
      return "The hold changed while you were collecting it. Refresh the dashboard and try again.";
    case 412:
      return "This hold can no longer be collected: it was already collected, cancelled or has expired.";
    default:
      return error.message;
  }
}

