import type { Metadata } from "next";
import { redirect } from "next/navigation";

import { uploadStockCsvAction } from "@/app/pharmacy-portal/actions";
import { Notices, queryText, type SearchParams } from "@/components/portal/Notices";
import { DashboardApiError, getSyncJob, type SyncJob } from "@/lib/portal/api";
import { portalSession } from "@/lib/portal/data";
import { CSV_HEADER, jobSummary } from "@/lib/portal/stock-csv";

export const metadata: Metadata = { title: "Upload stock CSV" };

type PageProps = { searchParams: SearchParams };

function ImportReport({ job }: { job: SyncJob }) {
  const summary = jobSummary(job);
  const rowErrors = [...(job.rowErrors ?? [])].sort((a, b) => (a.rowNumber ?? 0) - (b.rowNumber ?? 0));

  return (
    <section className="panel" aria-labelledby="report-heading">
      <div className="panel-heading">
        <h2 id="report-heading">Import report: {job.filename ?? "uploaded file"}</h2>
      </div>

      <p className={`message message-${summary.tone}`} role="status">
        {summary.label}
      </p>

      <div className="stat-grid" aria-label="Import totals">
        <div className="stat">
          <span className="stat-value num">{job.rowsTotal ?? 0}</span>
          <span className="stat-label">Rows in file</span>
        </div>
        <div className="stat">
          <span className="stat-value num">{job.rowsApplied ?? 0}</span>
          <span className="stat-label">Imported</span>
        </div>
        <div className="stat">
          <span className="stat-value num">{job.rowsRejected ?? rowErrors.length}</span>
          <span className="stat-label">Rejected</span>
        </div>
      </div>

      {rowErrors.length > 0 ? (
        <div className="table-wrap">
          <table>
            <caption className="sr-only">Rejected rows and the reason for each</caption>
            <thead>
              <tr>
                <th scope="col">Row</th>
                <th scope="col">Product ID</th>
                <th scope="col">Reason</th>
              </tr>
            </thead>
            <tbody>
              {rowErrors.map((row, index) => (
                <tr key={`${row.rowNumber ?? "x"}-${index}`}>
                  <td>{row.rowNumber ?? "—"}</td>
                  <td>
                    <code className="id-value">{row.productId || "—"}</code>
                  </td>
                  <td>{row.message ?? "Rejected"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </section>
  );
}

export default async function UploadPage({ searchParams }: PageProps) {
  const session = await portalSession();
  if (!session) redirect("/login?next=%2Fpharmacy-portal");

  const params = await searchParams;
  const jobId = queryText(params.job);
  let error = queryText(params.error);

  let job: SyncJob | undefined;
  if (jobId) {
    try {
      job = await getSyncJob(session, jobId);
    } catch (cause) {
      error = cause instanceof DashboardApiError ? cause.message : "Could not load the import report.";
    }
  }

  return (
    <div className="page">
      <div className="page-heading">
        <h1>Upload stock CSV</h1>
        <p className="muted">Update many medicines at once from a spreadsheet.</p>
      </div>

      <Notices error={error} />

      <section className="panel" aria-labelledby="upload-heading">
        <div className="panel-heading">
          <h2 id="upload-heading">Choose a file</h2>
          <a className="text-link" href="/pharmacy-portal/upload/template" download>
            Download CSV template
          </a>
        </div>

        <p className="csv-help">
          The first line must be <code>{CSV_HEADER}</code>. <code>quantity</code> is the total on hand
          (a whole number, 0 or more). <code>price_chhertum</code> is optional: leave it blank to keep the
          current price. UTF-8, at most 1 MB. Valid rows are applied; any rejected rows are listed with a
          reason.
        </p>

        <form action={uploadStockCsvAction} className="upload-form">
          <label className="sr-only" htmlFor="file">
            Stock CSV file
          </label>
          <input id="file" name="file" type="file" accept=".csv,text/csv" required />
          <button className="button button-primary" type="submit">
            Upload
          </button>
        </form>
      </section>

      {job ? <ImportReport job={job} /> : null}
    </div>
  );
}
