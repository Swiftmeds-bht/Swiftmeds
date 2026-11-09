# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

Patients and the family members who fetch medicine for them, in Bhutan (seeded towns: Thimphu, Paro, Phuentsholing). All of these are confirmed audiences:

- People on a phone, often unwell, tired or worried, deciding whether a trip to a pharmacy is worth it.
- Older patients and people with limited English or limited experience with apps. Plain words and large, steady type matter for them.
- People at a laptop or desktop, for example someone organising medicine for a relative.

Their job: find out which nearby pharmacy has a medicine in stock, hold it, and collect it at the counter with a pickup code.

## Product Purpose

SwiftMed shows which nearby pharmacies have a medicine in stock right now, so a patient does not travel from pharmacy to pharmacy. A patient can reserve units (hold them) and collect them with a pickup code before the hold expires. It is a success when the patient leaves home knowing where to go, and the medicine is waiting when they arrive.

## Positioning

Stock is reported per pharmacy and comes with a freshness signal (how recently it was confirmed). Results are ranked by distance, freshness and pharmacy reliability. When no pharmacy has the medicine, the app offers other brands with the same molecule and strength. A hold turns that stock answer into a promise the pharmacy keeps for a fixed time.

## Operating Context

- Search: medicine name (with suggestions), a town or the device location, and a radius. It is a plain GET form, so every search is a shareable URL and works without JavaScript.
- Results: products with their in-stock pharmacies (distance, price in Ngultrum, quantity, freshness), and same-molecule alternatives.
- Reserve: 1 to 5 units, never more than are in stock, after signing in.
- Hold lifecycle: Pending ("Checking stock", polled every 2 s), Active (pickup code plus a countdown in Bhutan time), Rejected, Expired, Cancelled, Collected.
- Account: sign in, register, My holds (current and past).
- The patient shows the pickup code at a pharmacy counter, so it must be readable at arm's length and read out digit by digit.

## Capabilities and Constraints

- Next.js App Router, server components, server actions. The access token lives in an httpOnly cookie, and gateway calls happen on the server.
- Demo mode runs without `GATEWAY_URL`, using `lib/mock-data.ts` and in-memory holds. It must always say that it shows demo data.
- If the gateway fails, the app shows an error. It never replaces a failed search with demo stock.
- Out of scope: payments, prescriptions, controlled drugs, POS integration, native apps.
- Routes, form field names and URL parameters are fixed: other services, tests and shared links depend on them.

## Brand Commitments

- The name "SwiftMed" is fixed. Logo and typefaces are open (confirmed 2026-11-09).
- Teal is the brand colour: the user asked to keep the teal green (#0f766e family) on 2026-11-09.
- Standing preference (2026-11-09): the patient app follows the category standard, a clean clinical health app, executed at the craft level of named reference products. It does not use a themed "world".
- Footer disclaimer, kept in substance: SwiftMed shows availability only. It does not give medical advice.
- Voice: plain, calm, factual sentences. No hype and no medical claims.

## Evidence on Hand

- Demo catalogue, pharmacies and stock: `lib/mock-data.ts`, built from `deploy/seed/`. These are not real pharmacies, prices or stock, and they must stay labelled as demo.
- There are no testimonials, partner logos, user numbers or press. Do not invent any.

## Product Principles

1. The answer comes first: where the medicine is, how far away, and how sure we are.
2. Honesty about certainty. Freshness, demo data, degraded services and failures are always stated, never hidden.
3. Works for someone unwell, on a cheap phone, on a slow network, without JavaScript.
4. A pickup code is a promise: it is unmistakable at the counter, and its time limit is always visible.
5. Never advise on medicine. Alternatives always come with "ask the pharmacist".

## Accessibility & Inclusion

WCAG 2.1 AA is required and tested: axe-core in `components/*a11y.test.tsx`, and token contrast in `lib/contrast.test.ts`. Touch targets are at least 44 px. The layout works at 320 px wide. Status changes use live regions. Users include older and low-literacy people, so keep body text large, labels explicit and icons paired with words.
