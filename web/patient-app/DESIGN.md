---
name: SwiftMed patient app
description: Find which nearby pharmacy in Bhutan has your medicine, hold it, and collect it with a code.
colors:
  brand-teal: "#0f766e"
  brand-teal-deep: "#115e59"
  teal-mist: "#e2f2ee"
  night-teal: "#0b3b37"
  night-teal-muted: "#b9d6d0"
  ticket-mist: "#def3ee"
  page-ground: "#f4f8f7"
  surface-white: "#ffffff"
  quiet-fill: "#ecf3f1"
  hairline: "#dbe6e3"
  field-stroke: "#7d8f8b"
  ink: "#10201e"
  ink-muted: "#4a5d5a"
  fresh-green: "#146c43"
  fresh-green-bg: "#e6f5ec"
  aging-amber: "#8a5200"
  aging-amber-bg: "#fff4dc"
  stale-red: "#b42318"
  stale-red-bg: "#fdecea"
  notice-blue: "#0b5470"
  notice-blue-bg: "#e8f3f6"
  focus-ring: "#0b3b37"
  focus-on-night: "#ffffff"
typography:
  display:
    fontFamily: "Figtree, system-ui, sans-serif"
    fontSize: "clamp(2.125rem, 4.6vw, 3.375rem)"
    fontWeight: 700
    lineHeight: 1.06
    letterSpacing: "-0.035em"
  headline:
    fontFamily: "Figtree, system-ui, sans-serif"
    fontSize: "clamp(1.75rem, 4.2vw, 2.375rem)"
    fontWeight: 700
    lineHeight: 1.12
    letterSpacing: "-0.025em"
  title:
    fontFamily: "Figtree, system-ui, sans-serif"
    fontSize: "1.1875rem"
    fontWeight: 650
    lineHeight: 1.35
    letterSpacing: "-0.01em"
  body:
    fontFamily: "Figtree, system-ui, sans-serif"
    fontSize: "1.0625rem"
    fontWeight: 400
    lineHeight: 1.55
  label:
    fontFamily: "Figtree, system-ui, sans-serif"
    fontSize: "0.9875rem"
    fontWeight: 600
    lineHeight: 1.5
  code:
    fontFamily: "Figtree, system-ui, sans-serif"
    fontSize: "clamp(2.375rem, 11vw, 3.375rem)"
    fontWeight: 750
    lineHeight: 1.15
    letterSpacing: "0.06em"
    fontFeature: "tnum, lnum"
rounded:
  control: "10px"
  panel: "16px"
  pill: "999px"
spacing:
  gutter: "20px"
  field-gap: "16px"
  panel-pad: "20px"
  section: "64px"
components:
  button-primary:
    backgroundColor: "{colors.brand-teal}"
    textColor: "{colors.surface-white}"
    rounded: "{rounded.control}"
    padding: "9px 22px"
    height: "44px"
  button-primary-hover:
    backgroundColor: "{colors.brand-teal-deep}"
    textColor: "{colors.surface-white}"
  button-outline:
    backgroundColor: "{colors.surface-white}"
    textColor: "{colors.brand-teal}"
    rounded: "{rounded.control}"
    height: "44px"
  button-outline-hover:
    backgroundColor: "{colors.teal-mist}"
    textColor: "{colors.brand-teal-deep}"
  input:
    backgroundColor: "{colors.surface-white}"
    textColor: "{colors.ink}"
    rounded: "{rounded.control}"
    height: "48px"
  chip-status-ready:
    backgroundColor: "{colors.fresh-green-bg}"
    textColor: "{colors.fresh-green}"
    rounded: "{rounded.pill}"
  chip-strength:
    backgroundColor: "{colors.teal-mist}"
    textColor: "{colors.brand-teal-deep}"
    rounded: "{rounded.pill}"
  panel:
    backgroundColor: "{colors.surface-white}"
    rounded: "{rounded.panel}"
    padding: "20px"
  pickup-ticket:
    backgroundColor: "{colors.brand-teal}"
    textColor: "{colors.surface-white}"
    typography: "{typography.code}"
    rounded: "{rounded.panel}"
---

# Design System: SwiftMed patient app

## Overview

**Creative North Star: "The Calm Pharmacist"**

SwiftMed behaves like a good pharmacist at the counter: clear, kind and exact. Every screen answers the patient's question first (where the medicine is, how far away, how sure we are), in plain words and steady type, and keeps everything else quiet. It is the category standard for a consumer health app, executed at the craft level of Zocdoc and One Medical, with SwiftMed's teal as its one voice.

The system is light, open and unhurried. White working panels sit on a page ground faintly tinted from the teal. A single deep-teal band greets the patient on the home page. Colour carries meaning, never decoration: teal for brand and action, green, amber and red only for stock freshness and hold state. The pickup ticket is the one signature object: the moment the pharmacy's promise becomes something the patient can hold up at the counter.

The user chose this canon direction on purpose over a themed visual world, so convention is the commitment. It is not an excuse for template defaults: no centred-hero-on-white, no equal feature cards, no dot badges, no eyebrow labels.

**Key Characteristics:**
- One accent (teal); state colours only where they mean something.
- Figtree throughout, with tabular figures for distance, price, stock, code and time.
- Labels always visible above fields; large body text (17px) for older readers.
- Two radii: 10px controls, 16px panels; chips are full pills.
- Soft depth tinted toward the teal; one authored motion (the code settling into place).

## Colors

A restrained palette: teal-tinted neutrals with one brand teal, and a small set of meaning colours.

### Primary
- **Brand Teal** (#0f766e): primary buttons, links, the pickup ticket, step numbers, focus of attention. The colour the user asked to keep.
- **Deep Teal** (#115e59): hover and pressed state for teal, the Best match label, strength chips.
- **Teal Mist** (#e2f2ee): the best-match row, strength chips, outline-button hover, the note under the home steps.
- **Night Teal** (#0b3b37): the home band only. Its text is white (headline) and Night Teal Muted (#b9d6d0) for supporting copy.

### Neutral
- **Page Ground** (#f4f8f7): the page behind every panel.
- **Surface White** (#ffffff): panels, inputs, the header and footer.
- **Quiet Fill** (#ecf3f1): hover fills, disabled selects, skeleton blocks.
- **Hairline** (#dbe6e3): panel borders and row dividers.
- **Field Stroke** (#7d8f8b): input and select borders (3:1 against white).
- **Ink** (#10201e) and **Ink Muted** (#4a5d5a): text and secondary text.

### Meaning colours
- **Fresh Green** (#146c43 on #e6f5ec): stock confirmed recently; hold ready to collect.
- **Aging Amber** (#8a5200 on #fff4dc): stock confirmed a while ago; hold checking stock.
- **Stale Red** (#b42318 on #fdecea): stock not confirmed lately; expired or rejected hold; errors; Cancel hold.
- **Notice Blue** (#0b5470 on #e8f3f6): informational notices such as "Showing demo data".

### Named Rules
**The One Voice Rule.** Teal is the only accent. Green, amber, red and blue appear only to state freshness, hold state, an error or a notice, never as decoration.

**The Tested Pair Rule.** Every text and colour pairing is listed in `lib/contrast.test.ts` and must pass WCAG AA. A new token is not done until its pairs are added there.

## Typography

**Body Font:** Figtree (with system-ui, -apple-system, Segoe UI, Roboto fallback), loaded with `next/font`.

**Character:** A warm, open humanist sans that stays friendly at display size and highly legible at 17px for older readers. Weight steps (400, 600, 650, 700, 750) and size do the hierarchy, never a second family.

### Hierarchy
- **Display** (700, clamp(2.125rem, 4.6vw, 3.375rem), 1.06, -0.035em): the home headline only, at most 2 lines on desktop.
- **Headline** (700, clamp(1.75rem, 4.2vw, 2.375rem), 1.12): page titles (Results for "…", Reserve, My holds).
- **Title** (650, 1.125–1.25rem, 1.3–1.35): product names, section headings, step names.
- **Body** (400, 1.0625rem, 1.55): all reading text; measure kept under about 60ch.
- **Label** (600, 0.9875rem, 1.5rem line): field labels, nav links, button text.
- **Code** (750, clamp(2.375rem, 11vw, 3.375rem), 0.06em tracking, tabular): the pickup code only.

### Named Rules
**The Counted Figures Rule.** Every number a patient compares or reads out (distance, price, stock, code, time left) is set in tabular lining figures.

**The No Eyebrow Rule.** Headings speak for themselves. No small uppercase tracked labels above them.

## Layout

The page frame is a 72rem (1152px) container with a 20px gutter for header, footer and the home band. Working pages use a 54rem column (results), narrowing to 30rem for forms and sign-in, and 34rem for a single hold, so lines stay short and the task stays in one column. The home band splits 1.25fr / 0.75fr above 62rem (headline and search left, example ticket right) and stacks below it, hiding the example. On the home page the search form is one column on phones, then puts Near, Within and Search in a row from 40rem. On the results page it is one compact bar at every width (medicine field and Search in a row) with the area folded into a native details line ("Near Thimphu, within 5.0 km · Change"), so the first pharmacy shows in the first phone screen. Result rows put the facts on the right (price, stock, Reserve) and stack below 32rem. Spacing rhythm: 16px between fields, 20px panel padding, 64px between home sections, more space above a heading than below it.

## Elevation & Depth

Panels are lifted gently off the tinted ground with soft shadows tinted toward Night Teal, never pure black. Depth tells the patient what they can act on: panels and the pickup ticket are lifted; rows inside a panel are separated by hairlines, not by more cards.

### Shadow Vocabulary
- **Resting** (`box-shadow: 0 1px 2px rgba(11, 59, 55, 0.06)`): compact search panel, hold rows in My holds, outline buttons.
- **Panel** (`box-shadow: 0 1px 2px rgba(11, 59, 55, 0.05), 0 10px 28px -14px rgba(11, 59, 55, 0.22)`): product results, forms, the hold card.
- **Lifted** (`box-shadow: 0 2px 4px rgba(11, 59, 55, 0.06), 0 18px 40px -18px rgba(11, 59, 55, 0.32)`): the suggestions list.
- **Ticket** (`box-shadow: 0 14px 30px -18px rgba(11, 59, 55, 0.7)`): the pickup ticket.

### Named Rules
**The No Nested Cards Rule.** A panel never holds another panel. Inside a panel, group with hairline dividers and tinted rows.

## Shapes

Gently rounded and consistent: 10px on every control (buttons, inputs, selects, notices), 16px on every panel and the pickup ticket, full pills only for chips and the header Sign in link. The pickup ticket alone has a cut silhouette: two semicircular notches at its dashed tear line, filled with the colour behind it.

## Components

### Buttons
- **Shape:** gently rounded (10px), at least 44px tall; 52px for the home Search.
- **Primary:** Brand Teal fill, white label at 650 weight, faint inner highlight. One primary per row: the best-match Reserve is filled, the other rows' Reserve buttons are outline.
- **Hover / Focus / Active:** fill deepens to Deep Teal; 3px Night Teal focus outline with a 2px offset (white on the home band), so focus never adds a second accent; pressed moves down 1px.
- **Outline:** white with Field Stroke border and teal label; hover fills Teal Mist. **Danger outline** (Cancel hold) uses Stale Red text and a soft red border.
- **Link button:** teal underlined text with an icon (Use my location); in the header it reads as a plain nav link.

### Chips
- **Status:** full pill, tinted background and matching text in the meaning colour (Ready to collect, Checking stock, Expired, Cancelled). It leads the meta line under the hold's title, never above it. Only Checking stock carries a mark: a turning loader icon, because it is live (still under reduced motion).
- **Strength:** Teal Mist pill with Deep Teal text next to a product name.

### Cards / Containers
- **Corner Style:** 16px.
- **Background:** Surface White on Page Ground.
- **Shadow Strategy:** Panel shadow (see Elevation).
- **Border:** 1px Hairline.
- **Internal Padding:** 20px (1.25rem), up to 28px on detail pages.

### Inputs / Fields
- **Style:** white, 1px Field Stroke, 10px radius, 48px tall (52px for the home medicine field), 17–18px text; label above in Label style, hint below in Ink Muted. Selects carry a drawn chevron.
- **Focus:** border turns Brand Teal plus a 3px Night Teal ring.
- **Error / Disabled:** errors sit below in a Stale Red tinted block; a disabled select gets Quiet Fill and a dashed border.

### Navigation
- **Header:** sticky, white with a slight blur, 64px tall: brand mark and SwiftMed wordmark, a muted tagline (hidden below 44rem), then My holds and Sign out as plain links, or Sign in as a bordered pill.

### Detail header
A detail page (a pharmacy) opens with a 48px Teal Mist tile holding a teal icon, beside the title and a muted one-line description. The actions under it share a row and an equal width: one filled, one outline.

### Totals
Money the patient will pay is shown live in a Teal Mist row: the label on the left ("To pay at the counter"), the amount on the right in bold tabular figures. It updates as the quantity changes.

### Empty and problem states
Both are solid white panels, never dashed boxes. A problem panel carries a red-tinted round icon, a title, the recovery in plain words and one action. Either one straight under a page title gets 20px of space above it.

### Freshness ramp (signature)
Three short bars like a signal meter, filled 3, 2 or 1 by freshness level, in the meaning colour, followed by the words ("Stock confirmed recently"). The bar count reads without colour.

### Pickup ticket (signature)
A Brand Teal ticket: "Pickup code" in Ticket Mist, the code in Code style with spaces between digits (read digit by digit to screen readers), a dashed tear line with notches, and the countdown below ("29:41 left until 14:30"). A compact version sits in My holds and in the home example. Only when a hold has just turned active (its status changed within the last 15 seconds), the ticket settles and the digits rise into place one after another (0.7s exponential ease-out, 70ms apart), starting from visible; reloads and lists never replay it, and reduced motion shows it still.

## Do's and Don'ts

### Do:
- **Do** answer first: distance, price, stock and freshness are visible on every result without opening anything.
- **Do** keep labels visible above every field and pair every icon with words.
- **Do** keep touch targets at least 44px and body text at 17px.
- **Do** mark demo data, degraded services and errors in a notice, every time.
- **Do** add new colour pairs to `lib/contrast.test.ts` before using them.

### Don't:
- **Don't** add a second accent colour or use green, amber or red decoratively.
- **Don't** use eyebrow labels, section numbers as decoration, dot badges, or equal icon-card grids.
- **Don't** put a panel inside a panel.
- **Don't** add a dark theme without a use-scene reason; the code is shown at a counter in daylight.
- **Don't** use placeholder text as a label, or text smaller than 15px for anything a patient must read.
