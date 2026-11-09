---
version: 1
slug: "app"
primary_target: "app"
related_targets: ["components"]
---

Scope: every route of web/patient-app (home, results, pharmacy, reserve, hold, My holds, sign in, register, not found, error). Visitor mode: Operate.

Audience and job: patients and carers in Bhutan, on phones and desktops, including older and low-literacy users. They find which nearby pharmacy has a medicine in stock, hold it, and collect it with a pickup code. Proof is the live stock answer itself: distance, price in Nu, quantity, freshness. There are no testimonials or claims. Constraints: routes, field names and URL parameters stay fixed, WCAG 2.1 AA, no-JS search keeps working, demo data stays labelled.

Craft bar: Zocdoc and One Medical (named by the user on 2026-11-09).

## Direction contract

THESIS: SwiftMed is a warm, trustworthy consumer-health app at Zocdoc / One Medical craft. The answer comes first: which pharmacy, how far, how sure. It refuses the template version of itself: a centred hero on white, equal feature cards, dot badges, eyebrow labels.

OWN-WORLD: Brand teal #0f766e with a deep teal ink band (#0b3b37), teal-tinted neutrals (#f3f7f6), white working surfaces. Teal is the only accent; green, amber and red appear only as stock-freshness and hold-state meaning. Figtree everywhere, with tabular figures for distance, price, code and time. Radii: controls 10px, panels 16px, chips full pill. Soft shadows tinted toward teal.

STORY: In one glance the patient sees who has the medicine, how far away it is, and how recently stock was confirmed. They reserve in two steps and show a code at the counter that cannot be misread.

FIRST VIEWPORT: Home: a full-width deep-teal band. On the left, a two-line headline and one sentence, with the search panel (medicine, Near, Within, Search) on a white card inside the band. On desktop the right column holds an example hold ticket, labelled Example. Below the band: the three-step search, reserve, collect sequence as one row. Results: a compact search bar, a plain summary sentence, the best-match pharmacy row raised, then the rest.

SIGNATURE: The pickup ticket. An active hold's code sits on a teal ticket with a notched tear line, digits in large tabular figures, and the countdown underneath. The one authored motion: when a hold turns active, the code digits rise into place in sequence (reduced motion: no movement).

FORM: Category standard (canon), chosen by the user over the rolled Recharge Voucher. seed:21ec087a5b638b8a

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance
