# JobRadar Design Specification

_Captured from live app — May 23, 2026_

Use this document to guide design work, UI consistency reviews, and feature additions. All tokens, components, and layouts are extracted from the running application.

---

## 1. Design Language

### Theme

- **Mode:** Dark-only UI
- **Personality:** Dense data dashboard — minimal decoration, information-forward, terminal-inspired
- **Brand mark:** "Job**Radar**" — "Job" in light weight, "Radar" in bold; followed by a `beta` badge in muted text

---

## 2. Color Tokens

### Background Layers

| Role                     | Approximate Value                      |
| ------------------------ | -------------------------------------- |
| App background           | `#0a0a0f` (near-black, very dark navy) |
| Sidebar background       | `#0f0f18` (slightly lighter than bg)   |
| Card / panel surface     | `#13131e`                              |
| Table row hover          | `#1a1a28`                              |
| Input / field background | `#1a1a28`                              |
| Detail panel background  | `#111118`                              |

### Border

| Role              | Approximate Value                 |
| ----------------- | --------------------------------- |
| Subtle border     | `#1e1e2e` (1px, almost invisible) |
| Active row border | `#2a2a40`                         |

### Brand / Primary

| Role                                           | Color                      |
| ---------------------------------------------- | -------------------------- |
| Primary accent (nav active, CTA bg, toggle on) | `#6c63ff` (violet-purple)  |
| Primary text on accent                         | `#ffffff`                  |
| Sidebar nav active background                  | `rgba(108, 99, 255, 0.15)` |
| Sidebar nav active left border                 | `#6c63ff` 2px              |

### Semantic Colors

| Meaning                   | Color                     |
| ------------------------- | ------------------------- |
| Success / OK / Remote tag | `#00d4aa` (teal-green)    |
| Warning / Offer pending   | `#f59e0b` (amber)         |
| Error / scan error        | `#ef4444` (red)           |
| New/active badge bg       | `#6c63ff` (purple)        |
| Salary text               | `#10b981` (emerald green) |
| "New" job status pill     | purple/violet             |
| On-site location dot      | gray                      |

### Text

| Role                    | Approximate Value |
| ----------------------- | ----------------- |
| Primary text            | `#e8e8f0`         |
| Secondary / muted text  | `#6b6b80`         |
| Caption / label         | `#4a4a5a`         |
| Link / interactive text | `#6c63ff`         |
| Destructive             | `#ef4444`         |

### Chart Colors

| Purpose                               | Color                                               |
| ------------------------------------- | --------------------------------------------------- |
| Activity heatmap (GitHub-style) — low | `#1e2a1e`                                           |
| Activity heatmap — mid                | `#2d5a3d`                                           |
| Activity heatmap — high               | `#00d4aa`                                           |
| Bar chart — primary series            | `#6c63ff` (purple)                                  |
| Bar chart — secondary series          | `#00d4aa` (teal)                                    |
| Line chart — primary                  | `#6c63ff`                                           |
| Scan health bar — OK                  | `#00d4aa`                                           |
| Scan health bar — Error               | `#ef4444`                                           |
| Tech stack bar chart                  | `#7c3aed` (violet)                                  |
| Role categories bars                  | multi-color: purple, orange, teal, green, red, blue |

---

## 3. Typography

### Font Family

- **UI font:** System sans-serif stack (appears to be Inter or similar geometric sans)
- **Mono / code:** Monospace for adapter names, scan intervals, action strings in audit log

### Scale

| Token        | Size  | Weight | Usage                                    |
| ------------ | ----- | ------ | ---------------------------------------- |
| `display`    | ~32px | 700    | Large KPI numbers (6890, 628)            |
| `heading-lg` | ~20px | 600    | Page title ("Dashboard", "Jobs")         |
| `heading-md` | ~14px | 600    | Section labels ("MY PROGRESS", "MARKET") |
| `heading-sm` | ~11px | 500    | Column headers (ALL CAPS, tracked)       |
| `body`       | ~13px | 400    | Table rows, body text                    |
| `caption`    | ~11px | 400    | Meta text, timestamps, counts            |
| `label`      | ~10px | 500    | Pill/badge labels, adapter chips         |

### Letter Spacing

- Section headers use `letter-spacing: 0.08em` (spaced-out caps, e.g. "MY PROGRESS", "TOP COMPANIES BY VELOCITY")

---

## 4. Spacing & Layout

### Page Shell

- **Sidebar width (expanded):** ~192px
- **Sidebar width (collapsed):** ~48px (icon-only, via `<<` toggle)
- **Content area:** fills remaining width
- **Content padding:** ~24px horizontal, ~24px vertical

### Grid

- **Dashboard:** 2-column grid (left 58%, right 42% approx)
- **Jobs list:** full-width table with fixed sidebar detail panel (~380px) that slides in on row click
- **Companies list:** full-width table with fixed sidebar detail panel (~380px)
- **Insights:** full-width single column with 2-column sub-grids for charts

### Component Spacing

- Card padding: `16–20px`
- Table row height: `48px`
- Section gap: `16px`
- Between cards: `12–16px`

---

## 5. Component Inventory

### Navigation (Sidebar)

```
[Logo: JobRadar beta]     [<< collapse]
─────────────────────
[icon] Dashboard          ← active state: purple bg + left border
[icon] Jobs
─────────────────────
ADMIN
[icon] Companies
[icon] Users
[icon] Audit Log
[icon] Insights
[icon] Settings
─────────────────────
[T avatar] email@...  [→ logout]
```

- Active item: `bg rgba(108,99,255,0.15)`, `border-left: 2px solid #6c63ff`, text white
- Inactive: muted text, icon-only in collapsed state

### KPI Card

```
┌─────────────────────────────────┐
│ CARD LABEL (caps, muted)        │
│                                 │
│  METRIC_A   METRIC_B  METRIC_C  │
│  [value]    [value]   [value]   │
│  [large]    [large]   [large]   │
│  sub-label  sub-label sub-label │
└─────────────────────────────────┘
```

- Dark card surface, 8px border-radius
- Used for "My Progress" (Applied/Interviewing/Offer) and "Market" (New Jobs / Total Open)
- Large numbers use `display` type scale; colored by meaning (white, amber, purple, teal)

### Activity Heatmap (GitHub-style)

- 2-row grid of small squares (~8×8px), representing weeks/months
- Color scale from dark (no activity) → teal (high activity)
- "fewer / more" legend with color gradient below
- Used on Dashboard "New Jobs Detected" panel

### Data Table

```
[ ] Company ↑  Title ↑  Location    Salary      Added ↑
─────────────────────────────────────────────────────────
[ ] [Avatar] CompanyName  Job Title  [tag]  $XXXk–$XXXk  Xh ago
```

- Checkbox column (multi-select)
- Company avatar: colored square with first letter, ~28px, 6px radius
- Location shown as pill tag (Remote = teal pill, On-site = gray dot + text)
- Salary in emerald green
- Timestamps in muted caption text
- Sortable columns with up/down arrow icon
- Pagination bar at bottom: "Showing 1-N of M · Prev · Page N · Next · Rows [dropdown]"

### Job Detail Panel (Slide-in)

```
[trash] [x close]
[Avatar] Job Title
         Company Name
[5+ yrs exp] [Remote] [full time] [Sponsors visa]
[Open posting ↗] [New ▾]

Added Xh ago

SUMMARY
...paragraph...

RESPONSIBILITIES
• bullet
• bullet

TECH STACK
[React] [JavaScript] [CSS] [Python] [Flask] ...

YOUR NOTES
[Private notes textarea]
```

- Fixed right panel, ~380px wide
- Overlaps table, pushes table narrower
- Tags are small rounded pills: experience level (gray), Remote (teal), type (gray), visa (blue)
- Status dropdown `[New ▾]` with chevron

### Status Pill / Tag

| Variant                                                          | Style                          |
| ---------------------------------------------------------------- | ------------------------------ |
| Remote                                                           | Teal bg, teal text, `⇥` icon   |
| On-site                                                          | Gray dot + text                |
| OK                                                               | Small green dot + "OK" text    |
| Error                                                            | Small red dot + "Error" text   |
| Running                                                          | Animated dot                   |
| Queued                                                           | Gray dot                       |
| `gem`, `ashby`, `lever`, `greenhouse`, `workday` (adapter chips) | Dark pill, mono font, no color |

### Toggle Switch

- On: purple track (`#6c63ff`), white thumb
- Off: dark gray track
- Size: ~36×20px

### Button Variants

| Variant          | Style                                                            |
| ---------------- | ---------------------------------------------------------------- |
| Primary CTA      | `+ Add Company` — purple bg, white text, 6px radius              |
| Secondary        | `Run all`, `Deactivate all` — dark bg, white text, subtle border |
| Ghost / icon     | Icon only, no bg, hover shows bg                                 |
| Scan now         | Purple outline with text                                         |
| Destructive icon | Red trash icon (no bg)                                           |

### Filter Bar (Jobs page)

```
[🔍 Search...]  [All companies ▾]  [0-2 yrs] [2-5 yrs] [5+ yrs] [24h] [7d] [30d] [Remote] [On-site]
```

- Search input with magnifier icon, dark bg, rounded
- Dropdown filter pill: dark bg, subtle border, chevron
- Toggle filter pills: dark bg, rounded, active = slightly brighter bg

### Company Pipeline Status Bar (Companies page)

```
1208         0         0         228              0           0          520
ACTIVE  →  QUEUED  →  CRAWLING  →  AWAITING   →  FILTER   →  AWAITING  →  NEW JOBS
                                   FILTER         RETRYING   ENRICHMENT     (24H)
```

- Horizontal flow diagram showing pipeline stages
- Numbers above, labels below
- Arrows between stages
- Key metric "520 NEW JOBS (24H)" highlighted in teal/accent

### Scan Health Sparkline (Companies list)

- "Last 10 Scans" column shows a mini bar chart of 10 bars
- OK = teal bars, Error = red bars
- Represents recent scan history at a glance

### Funnel Chart (Insights)

```
Enriched (total)  ████████████████████████  3074 (100%)
Saved             ████                        54 (2%)
Applied           █                            1 (0%)
Dismissed         ██                          30 (1%)
```

- Horizontal bar funnel, decreasing bar lengths
- Each bar a different color (purple, amber, green, red)

### Bar Charts (Insights)

- Role Categories: horizontal bars, color-coded by category
- Tech Stack Trends: horizontal bars, violet `#7c3aed`
- Location Breakdown: horizontal bars, purple / cyan

### Line Chart (Insights — Sponsorship Availability)

- Recharts line chart, purple line, dark grid lines, no fill

### Company Detail Panel (Companies page)

```
[1] 11x.ai              [Scan now] [edit] [trash] [x]
    jobs.gem.com · gem

RECENT SCANS
[bar sparkline 20 bars]   7h ago    578ms
                          LAST SCAN  AVG TIME
100% SUCCESS

Logs ▾

NEW JOBS
  0        14        14
 TODAY    30 DAYS   ALL TIME

[View 14 new jobs →]

HIRING ACTIVITY
[line chart — monthly]
```

### Audit Log Table

```
Time                Actor                Action                          Target
5/22/2026, 9:53 AM  email@example.com    POST /admin/companies           company
...
```

- Simple 4-column table
- Action column uses bold/highlight for the HTTP verb + path
- Filter inputs at top: "Actor user ID or email..." | "Action (e.g. user.delete)..."
- "483 entries" count top-right

### Users Table

```
Email              Status    Roles         Joined    Actions
email@example.com  [active]  [user][admin] 5/6/2026  [🔄] [🚫] [🗑]
```

- Status badge: `active` = green pill
- Role badges: `user` (gray), `admin` (purple)
- Actions: reset password icon, suspend icon, delete icon
- "you" tag next to own row

### Settings Table (Adapter Config)

```
● apple       720 min   5 s    Just now   5/3/2026
● ashby       720 min   3 s    Just now   5/4/2026
```

- Status dot colored by adapter type
- Inline editable fields (scan interval, delay)
- Monospace adapter name

---

## 6. Page Layouts

### Dashboard

```
Sidebar | [My Progress KPI card] [Market KPI card] [Activity Heatmap card]
        | [Interviewing panel]                      [Top Companies by Velocity]
        | [Tech Stack Trends — full width]
```

### Jobs

```
Sidebar | [Tab bar: New | Saved | Applied | Interviewing | Offer | Rejected]
        | [Filter bar: Search | Company dropdown | exp | time | location]
        | [Table: checkbox | Company | Title | Location | Salary | Added]
        |                                              [Job Detail Panel →]
        | [Pagination bar]
```

### Companies (Admin)

```
Sidebar | Companies  [1208 ACTIVE → 0 QUEUED → ... → 520 NEW JOBS]  [Live] [Refresh]
        | [Search] [All adapters ▾] [All|Active|Inactive] [OK|Errors|Running|Queued]
        |                           [Deactivate all] [Run all] [+ Add Company]
        | [Table: Company | Adapter | Status | Last 10 Scans | Last scan | Active]
        |                                              [Company Detail Panel →]
        | [Pagination]
```

### Users (Admin)

```
Sidebar | Users
        | [Filter by email...]  [All statuses ▾]          [N users]
        | [Table: Email | Status | Roles | Joined | Actions]
        | [Pagination]
```

### Audit Log (Admin)

```
Sidebar | Audit Log
        | [Actor filter]  [Action filter]                  [N entries]
        | [Table: Time | Actor | Action | Target]
        | [Pagination]
```

### Insights (Admin)

```
Sidebar | Insights
        | Pipeline analytics — based on all enriched jobs
        |
        | [Pipeline Funnel card]          [Role Categories card]
        | [Company Hiring Velocity card — full width]
        | [Location Breakdown — 2 col: By Country | USA by State]
        | [Sponsorship Availability — line chart]
        | [Tech Stack Trends — horizontal bar chart]
```

### Settings (Admin)

```
Sidebar | Settings
        | Adapter Configuration
        | Per-adapter scan intervals and inter-company delays.
        | [Table: Adapter | Scan Interval | Inter-Company Delay | Last Scheduled | Updated]
```

---

## 7. Interaction Patterns

### Row Click → Slide-in Panel

- Clicking a job row or company row opens a right-side detail panel
- Panel overlaps table; table stays scrollable on left
- Panel has `[x]` close button top-right
- Panel has contextual actions top-right (trash, edit, scan now)

### Tab Navigation (Jobs)

- Horizontal tab bar below page heading
- Active tab: white text + bottom border in accent color
- Badge counts on each tab (e.g. `New 6807`, `Saved 54`)

### Filter Pills

- Toggle on/off with click
- No submit button — filters apply immediately (query invalidation pattern)

### Pagination

- "Showing X-Y of Z" on left
- Prev / Page N / Next on center
- "Rows [20 ▾]" dropdown on right

### Empty States

- "No active applications" — centered muted text, no icon

### Admin Pipeline Counter

- Real-time numbers (with `[Live]` button to toggle live refresh)
- `[Refresh]` manual refresh button

---

## 8. Navigation Structure

```
/dashboard          → Dashboard
/jobs               → Jobs (New tab default)
/jobs?tab=saved     → Jobs Saved tab
/jobs?tab=applied   → Jobs Applied tab
/admin/companies    → Companies admin
/admin/users        → Users admin (route: /users or /admin/users)
/admin/audit-log    → Audit Log
/insights           → Insights
/settings           → Settings
```

---

## 9. Figma File Recommendations

When building the Figma file, structure it as:

### Pages

1. **🎨 Design Tokens** — color styles, text styles, spacing tokens
2. **🧩 Components** — all atoms and molecules (buttons, pills, tags, inputs, toggles, avatars, cards)
3. **📐 Templates** — page shell with sidebar, table template, detail panel template
4. **📱 Dashboard** — full Dashboard page mockup
5. **💼 Jobs** — Jobs list + detail panel
6. **🏢 Companies** — Companies admin list + detail panel
7. **👥 Users & Audit** — Users and Audit Log pages
8. **📊 Insights** — Insights page with all charts
9. **⚙️ Settings** — Settings page
10. **🔀 Flows** — Job status flow, navigation map

### Component Organization

```
Atoms/
  Colors          (swatches for all tokens)
  Typography      (text styles specimen)
  Icons           (sidebar icons, action icons)
  Badges          (status pills, role badges, adapter chips)
  Buttons         (primary, secondary, ghost, icon)
  Inputs          (search, filter, textarea)
  Toggle          (on/off)
  Avatar          (company letter avatar, user avatar)

Molecules/
  KPI Card
  Table Row (jobs, companies, users, audit)
  Filter Bar
  Pagination Bar
  Sidebar Nav Item
  Detail Panel Header
  Scan Sparkline
  Activity Heatmap Cell
  Pipeline Status Counter

Organisms/
  Sidebar (expanded + collapsed)
  Jobs Table
  Company Table
  Detail Panel (job, company)
  Funnel Chart
  Bar Chart Row
```

---

## 10. Suggested Claude Code Workflow

To use this spec effectively with Claude Code:

1. **Reference this file** at the top of any UI prompt: `"Follow the design spec in Designs/jobradar-design-spec.md"`
2. **For new components**, point Claude Code to the relevant section of this doc
3. **For color consistency**, always reference Section 2 — never hardcode colors
4. **For layout work**, reference Section 6 page layouts and Section 4 spacing
5. **Update this file** when new pages or components are added

---

## 11. Better Approaches for Figma → Code Workflow

### Option A: Design Tokens JSON (Recommended addition)

Extract all color/type/spacing tokens into a `design-tokens.json` (W3C DTCG format) and import into both Figma (via Tokens Studio plugin) and the codebase. This creates a single source of truth.

### Option B: Figma → Code via Locofy / Builder.io

After building Figma frames, use Locofy or Builder.io to generate React+Tailwind code skeletons. Feed those into Claude Code to refine against the actual codebase patterns.

### Option C: Storybook Component Catalog

Run `storybook` alongside the dev app. Screenshots of Storybook stories become the canonical design reference for Claude Code — more precise than Figma for code-level decisions.

### Option D: Screenshots in CLAUDE.md (Current best practice)

Add a `ui/console/CLAUDE.md` section that links to this spec file and adds a note that Claude should reference it when building or modifying UI components. Claude Code's context loading will pick it up automatically when working in the UI directory.

### What to put in the Figma file

Focus Figma on **design intent** (spacing, color decisions, new feature mockups) rather than pixel-perfect replicas of existing screens. Use Auto Layout aggressively so designs stay responsive. Keep the Figma file as a **design decision log**, not a mirror of the code.
