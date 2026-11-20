import { fetchSuggestions } from "@/lib/api";

// The search box asks this same-origin route for name suggestions, and the route asks
// the gateway. That keeps the gateway address on the server and avoids browser CORS.
export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  const prefix = new URL(request.url).searchParams.get("prefix") ?? "";
  const suggestions = await fetchSuggestions(prefix);
  return Response.json({ suggestions }, { headers: { "Cache-Control": "no-store" } });
}
