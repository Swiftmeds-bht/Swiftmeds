import { describe, expect, it } from "vitest";

import { CSV_HEADER, MAX_CSV_BYTES, csvFileProblem, csvTemplate, jobSummary } from "./stock-csv";

describe("csvFileProblem", () => {
  it("accepts a non-empty .csv up to 1 MB", () => {
    expect(csvFileProblem({ name: "stock.csv", size: 120 })).toBeUndefined();
    expect(csvFileProblem({ name: "STOCK.CSV", size: MAX_CSV_BYTES })).toBeUndefined();
  });

  it("refuses files Stock Sync would refuse outright", () => {
    expect(csvFileProblem(null)).toMatch(/choose a csv/i);
    expect(csvFileProblem({ name: "", size: 0 })).toMatch(/choose a csv/i);
    expect(csvFileProblem({ name: "stock.xlsx", size: 10 })).toMatch(/\.csv file/);
    expect(csvFileProblem({ name: "stock.csv", size: 0 })).toMatch(/empty/);
    expect(csvFileProblem({ name: "stock.csv", size: MAX_CSV_BYTES + 1 })).toMatch(/1 MB/);
  });
});

describe("csvTemplate", () => {
  it("is just the header without stock", () => {
    expect(csvTemplate()).toBe(`${CSV_HEADER}\r\n`);
  });

  it("is pre-filled with current quantities and blank prices", () => {
    const csv = csvTemplate([
      { pharmacyId: "ph", productId: "p-1", quantityOnHand: 12 },
      { pharmacyId: "ph", productId: "p-2" },
    ]);
    expect(csv.split("\r\n")).toEqual([CSV_HEADER, "p-1,12,", "p-2,0,", ""]);
  });

  it("quotes product IDs that would break the CSV", () => {
    expect(csvTemplate([{ pharmacyId: "ph", productId: 'a,"b"', quantityOnHand: 1 }])).toContain('"a,""b""",1,');
  });
});

describe("jobSummary", () => {
  it("describes each job status", () => {
    expect(jobSummary({ status: "SYNC_JOB_STATUS_COMPLETED", rowsApplied: 3 })).toEqual({
      label: "All 3 rows were imported.",
      tone: "success",
    });
    expect(jobSummary({ status: "SYNC_JOB_STATUS_PARTIAL", rowsApplied: 2, rowsRejected: 1 }).tone).toBe("warning");
    expect(jobSummary({ status: "SYNC_JOB_STATUS_PARTIAL", rowsApplied: 2, rowsRejected: 1 }).label).toMatch(
      /2 rows were imported and 1 were rejected/,
    );
    expect(jobSummary({ status: "SYNC_JOB_STATUS_FAILED" }).tone).toBe("error");
    expect(jobSummary({ status: "SYNC_JOB_STATUS_PROCESSING" }).tone).toBe("info");
    expect(jobSummary({ status: "SOMETHING_NEW" }).tone).toBe("info");
  });
});
