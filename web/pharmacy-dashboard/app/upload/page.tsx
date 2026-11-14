import Link from "next/link";
import { redirect } from "next/navigation";

import { logoutAction, uploadStockCsvAction } from "@/app/actions";
import { DashboardApiError, getSyncJob, type SyncJob } from "@/lib/api";
import { getSession } from "@/lib/session";
import { CSV_HEADER, jobSummary } from "@/lib/stock-csv";

type PageProps = {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
};

function queryText(value: string | string[] | undefined): string | undefined {
  return typeof value === "string" && value ? value : undefined;
}

function ImportReport({ job }: { job: SyncJob }) {
  const summary = jobSummary(job);
  const rowErrors = [...(job.rowErrors ?? [])].sort((a, b) => (a.rowNumber ?? 0) - (b.rowNumber ?? 0));

  return (
    <section className="panel" aria-labelledby="report-heading">
      <div className="panel-heading">
        <div>
          <p className="eyebrow">Import report</p>
          <h2 id="report-heading">{job.filename ?? "Uploaded file"}</h2>
        </div>
      </div>

      <p className={`message message-${summary.tone}`} role="status">
        {summary.label}
      </p>

      <div className="summary-grid" aria-label="Import totals">
        <article className="summary-card">
          <span className="summary-label">Rows in file</span>
          <strong className="summary-value">{job.rowsTotal ?? 0}</strong>
        </article>
        <article className="summary-card">
          <span className="summary-label">Imported</span>
          <strong className="summary-value">{job.rowsApplied ?? 0}</strong>
        </article>
        <article className="summary-card">
          <span className="summary-label">Rejected</span>
          <strong className="summary-value">{job.rowsRejected ?? rowErrors.length}</strong>
        </article>
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
  const session = await getSession();
  if (!session) redirect("/login");

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
    <main className="dashboard-shell">
      <header className="dashboard-header">
        <div>
          <p className="eyebrow">SwiftMeds · Pharmacy</p>
          <h1>Upload stock CSV</h1>
          <p className="dashboard-subtitle">Update many medicines at once from a spreadsheet.</p>
        </div>

        <div className="header-actions">
          <Link className="button button-secondary" href="/">
            Back to dashboard
          </Link>
          <form action={logoutAction}>
            <button className="button button-secondary" type="submit">
              Sign out
            </button>
          </form>
        </div>
      </header>

      {error ? (
        <p className="message message-error" role="alert">
          {error}
        </p>
      ) : null}

      <section className="panel" aria-labelledby="upload-heading">
        <div className="panel-heading">
          <div>
            <p className="eyebrow">Stock Sync</p>
            <h2 id="upload-heading">Choose a file</h2>
          </div>
          <a className="text-link" href="/upload/template" download>
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
    </main>
  );
}
