// Helpers for the stock CSV upload (SCRUM-70). The Stock Sync service does the
// real row validation; these checks only catch files it would refuse outright,
// so the pharmacist gets an answer without a round trip.
import type { StockLevel, SyncJob } from "./api";

/** Matches services/stock-sync/internal/domain.MaxCSVBytes. */
export const MAX_CSV_BYTES = 1 << 20;

export const CSV_HEADER = "product_id,quantity,price_chhertum";

interface FileLike {
  name: string;
  size: number;
}

/** Returns why the file cannot be uploaded, or undefined when it can. */
export function csvFileProblem(file: FileLike | null | undefined): string | undefined {
  if (!file || !file.name) return "Choose a CSV file to upload.";
  if (!/\.csv$/i.test(file.name)) return "The file must be a .csv file.";
  if (file.size === 0) return "The file is empty.";
  if (file.size > MAX_CSV_BYTES) return "The file is larger than 1 MB.";
  return undefined;
}

function csvField(value: string): string {
  return /[",\r\n]/.test(value) ? `"${value.replace(/"/g, '""')}"` : value;
}

/**
 * Builds the downloadable template. With the pharmacy's current stock it is
 * pre-filled, so the pharmacist only has to change the quantities. Prices are
 * left blank, which Stock Sync treats as "keep the current price".
 */
export function csvTemplate(stock: StockLevel[] = []): string {
  const rows = stock.map((item) => [csvField(item.productId), String(item.quantityOnHand ?? 0), ""].join(","));
  return [CSV_HEADER, ...rows].join("\r\n") + "\r\n";
}

export interface JobSummary {
  label: string;
  tone: "success" | "warning" | "error" | "info";
}

export function jobSummary(job: Pick<SyncJob, "status" | "rowsApplied" | "rowsRejected" | "rowsTotal">): JobSummary {
  const applied = job.rowsApplied ?? 0;
  const rejected = job.rowsRejected ?? 0;

  switch (job.status) {
    case "SYNC_JOB_STATUS_COMPLETED":
      return { label: `All ${applied} rows were imported.`, tone: "success" };
    case "SYNC_JOB_STATUS_PARTIAL":
      return {
        label: `${applied} rows were imported and ${rejected} were rejected. Fix the rejected rows and upload them again.`,
        tone: "warning",
      };
    case "SYNC_JOB_STATUS_FAILED":
      return { label: "No rows were imported. See the reasons below.", tone: "error" };
    case "SYNC_JOB_STATUS_PROCESSING":
      return { label: "The file is still being processed. Refresh this page in a moment.", tone: "info" };
    default:
      return { label: "The import finished with an unknown status.", tone: "info" };
  }
}
