import { ImageResponse } from "next/og";

// The link preview (SCRUM-157): what WhatsApp, Slack and slide tools show for
// a shared SwiftMed link. Rendered once at build time.

export const alt = "SwiftMed: find your medicine nearby in Bhutan, reserve it and collect it with a pickup code";
export const size = { width: 1200, height: 630 };
export const contentType = "image/png";

const TEAL = "#0b3b37";
const MINT = "#b9d6d0";

export default function OpengraphImage() {
  return new ImageResponse(
    (
      <div
        style={{
          width: "100%",
          height: "100%",
          display: "flex",
          flexDirection: "column",
          justifyContent: "space-between",
          padding: "72px 80px",
          background: `linear-gradient(135deg, ${TEAL} 0%, #0f766e 100%)`,
          color: "#ffffff",
          fontFamily: "sans-serif",
        }}
      >
        <div style={{ display: "flex", alignItems: "center", gap: 24 }}>
          <div
            style={{
              width: 88,
              height: 88,
              borderRadius: 24,
              background: "#ffffff",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
            }}
          >
            <svg width="56" height="56" viewBox="0 0 28 28">
              <path d="M14 6v16M6 14h16" stroke={TEAL} strokeWidth="4" strokeLinecap="round" />
            </svg>
          </div>
          <div style={{ fontSize: 64, fontWeight: 700, letterSpacing: -2 }}>SwiftMed</div>
        </div>

        <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
          <div style={{ fontSize: 76, fontWeight: 700, lineHeight: 1.05, letterSpacing: -2.5, maxWidth: 980 }}>
            Find your medicine before you leave home
          </div>
          <div style={{ fontSize: 34, color: MINT, maxWidth: 980 }}>
            Live stock from verified pharmacies across Bhutan. Reserve it, then collect it with a code.
          </div>
        </div>

        <div style={{ display: "flex", gap: 16, fontSize: 26, color: MINT }}>
          {["Search nearby", "Reserve", "Collect with a code"].map((step, i) => (
            <div key={step} style={{ display: "flex", alignItems: "center", gap: 16 }}>
              {i > 0 ? <div style={{ width: 8, height: 8, borderRadius: 8, background: MINT }} /> : null}
              <div>{step}</div>
            </div>
          ))}
        </div>
      </div>
    ),
    size,
  );
}
