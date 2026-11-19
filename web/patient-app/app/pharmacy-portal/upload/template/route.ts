import { listPharmacyStock } from "@/lib/portal/api";
import { portalSession } from "@/lib/portal/data";
import { csvTemplate } from "@/lib/portal/stock-csv";

// GET /upload/template: the stock CSV template. Signed in, it is pre-filled with
// the pharmacy's current stock; if stock cannot be loaded it is just the header.
export async function GET(): Promise<Response> {
  const session = await portalSession();
  let stock: Awaited<ReturnType<typeof listPharmacyStock>> = [];

  if (session) {
    try {
      stock = await listPharmacyStock(session);
    } catch {
      stock = [];
    }
  }

  return new Response(csvTemplate(stock), {
    headers: {
      "Content-Type": "text/csv; charset=utf-8",
      "Content-Disposition": 'attachment; filename="swiftmed-stock-template.csv"',
      "Cache-Control": "no-store",
    },
  });
}
