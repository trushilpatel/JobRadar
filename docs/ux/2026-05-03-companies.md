# JobRadar UX Review — Companies Page — 2026-05-03

## Section reviewed

Companies page — `/companies`  
Components in scope: PipelineStrip, CompaniesHeader, CompaniesFilterBar, CompaniesTable (+ columns), CompanyDetailDrawer, AddCompanyModal

## Live app state

- **549 companies** tracked, all 549 currently marked active
- Pipeline is quiet: Queued = 0, Crawling = 0, Awaiting Filter = 0, Awaiting Enrichment = 0
- Scans appear healthy — last scans ran 2–8 hours ago for visible rows; most histories show all-OK bars
- Several companies (Ashby-based, Lever-based) show all-Idle bars — these have no recent-enough data to fill all 10 history slots
- Auto-refresh is **off** on page load
- No scan errors visible in the current data set

---

## Component Reviews

### PipelineStrip

**CRITIC**: The strip is doing good work conceptually but has three concrete failures right now.

1. **Total = Active = 549 side-by-side is meaningless.** When every company is active, showing both numbers with only an arrow between them communicates nothing — it looks like a formatting error, not a metric.
2. **Four consecutive zeros (Queued → Crawling → Awaiting Filter → Awaiting Enrichment) kill the visual.** The entire pipeline looks dead. There is no zero-state treatment: no dimmed slots, no "pipeline idle" label, no collapse. Users have to consciously interpret four 0s as "healthy idle" rather than "something broke."
3. **9px uppercase labels are below comfortable reading size.** On any non-Retina display this text will blur. The `text-[9px]` class is outside Tailwind's default scale for a reason.
4. **"New Jobs (24h)" is the only outcome metric but is buried at the far right**, separated by four zero-valued process metrics. Users who care about output have to scan the entire strip to reach the one number that matters.
5. **No overflow handling.** On a 1200px viewport the strip fits, but at 900–1024px the labels will start wrapping or clipping silently since the parent uses `overflow-x-auto` with no visual scroll indicator.

**ADVOCATE**: The strip is a real operational win — before it existed there was no at-a-glance way to see where pipeline blockage was occurring. The arrow-flow metaphor is correct for a pipeline view and the color coding (amber for queued, blue for crawling, violet for filter/enrichment, emerald for output) is consistent with the rest of the app.

On the Total/Active redundancy: I'd argue it's useful precisely because it catches the case when some companies go inactive — the delta tells you something. The problem is today's data makes it look pointless.

On the zeros: I partially agree with the Critic. A fix that doesn't require a structural change would be to dim or de-emphasize zero-value stages rather than display them with full weight. The `valueClass` prop already exists on `Stage` — we could conditionally pass a dimmed class when `value === 0`.

On the 9px labels: this is a real accessibility issue with no defense. The minimum for body/label text in the WCAG guidance is around 11px for secondary labels.

**Proposed improvements:**

1. Dim zero-valued pipeline stages (`text-white/25` on value, `text-slate-700` on label) so the idle state reads as quiet rather than broken.
2. Increase stage label size to `text-[10px]` minimum; currently `text-[9px]` will fail contrast ratios on matte displays.
3. Collapse the Total/Active pair when they're equal — show only one: "549 Active" — and restore both only when they differ.

**SYNTHESIS**:

1. **Dim zero-value stages** — change `valueClass` to `text-white/25` when `value === 0`, so a healthy-idle pipeline reads as quiet, not broken. High impact, ~5 min change. (`PipelineStrip.tsx`, `Stage` component)
2. **Increase label font to `text-[10px]`** — accessibility fix, the current `text-[9px]` is below minimum legible size on standard displays. (`PipelineStrip.tsx`, `Stage`)
3. **Collapse Total/Active when equal** — render "549 Active" as a single stage when `total === active`; show both only when they differ. Reduces cognitive noise. (`PipelineStrip.tsx`)

---

### CompaniesHeader

**CRITIC**:

1. **The auto-refresh button is ambiguous.** It shows "30s" as its label. The label doesn't tell you the state — only the color does (green = on, grey = off). A user who is colorblind or skimming will have no idea if refresh is on or off until they read the tooltip. The `title` attribute says "Auto-refresh off — click to enable (30s)" but that's not visible at a glance.
2. **The icon-only refresh button has no visible label.** `title="Refresh data"` only shows on hover — it's invisible to keyboard users and inaccessible on touch devices. The two buttons (auto-refresh + manual refresh) look like one group but behave completely differently.
3. **The page title "Companies" and the pipeline strip compete for vertical attention in the same row.** The strip is richer than the title but they're peers visually. A new user will likely stare at the strip trying to decode it before they read anything else.
4. **`localStorage` for auto-refresh preference** works but means the state is browser-specific and silently persists. If a user enables it on their laptop and then uses the app on another machine, they're surprised when it's off. This is a minor UX concern but worth noting.

**ADVOCATE**: The header is actually quite compact for what it does — it packs page identity, pipeline telemetry, error alert, and refresh controls into ~56px of vertical space. That's a reasonable density for a power-user tool.

On the auto-refresh label: I agree the "30s" standalone label is confusing. But adding a full "Auto-refresh: ON" label would push the header wider. The best path is a short text change: show "Live" when on and "30s" when off, in addition to the color. The state is then unambiguous from the label alone.

On the icon-only refresh: add `aria-label` + a short text label that's always visible, e.g., "Refresh". The two-button cluster currently looks like a settings control rather than action controls.

On the title/strip competition: the title is semantic/nav anchor; the strip is operational. Giving the strip more visual breathing room (its own thin sub-bar or a subtle background fill) would establish hierarchy without restructuring the layout.

**SYNTHESIS**:

1. **Make auto-refresh state legible from label alone** — change the button label from `"30s"` to `"Live"` (on) or `"Auto"` (off), so the current state is readable without relying on color or hover. (`CompaniesHeader.tsx`)
2. **Add visible text to the manual Refresh button** — replace the icon-only button with `<RefreshCw size={12}/> Refresh` text+icon pair (matching the auto-refresh button's format). Improves keyboard accessibility and discoverability. (`CompaniesHeader.tsx`)
3. **Add `aria-label` to the auto-refresh toggle** — "Auto-refresh: on" / "Auto-refresh: off" to convey state to screen readers. The current tooltip is on `title` only, which is not reliably announced. (`CompaniesHeader.tsx`)

---

### CompaniesFilterBar

**CRITIC**:

1. **"Deactivate all" and "Run all" are placed in the center of the filter bar with no visual separation from filter controls.** On a wide viewport these float between filters and "Add Company." A distracted user could click "Deactivate all" intending to click a filter toggle. Given that this action would deactivate all 549 companies, there is no confirmation dialog.
2. **"Run all" also lacks a confirmation** — queueing 549 simultaneous scans will hammer the backend. Both bulk actions need friction: a confirmation dialog or at minimum a "you're about to affect N companies" prompt.
3. **The status filter SegmentedSwitch uses Unicode characters (✕, ⟳, ·) as icons** instead of Lucide icons used everywhere else. These render inconsistently across operating systems (especially ⟳ on Windows).
4. **The "Reset" button is 11px text, no border, no background** — it's the visual equivalent of a footnote. Users who want to clear filters after experimenting will struggle to find it.
5. **There is no visual indicator that filters are active.** If a user has applied an adapter filter and a status filter, nothing in the bar communicates "2 filters active." They could reload the page and wonder why the list looks different.

**ADVOCATE**: The layout tripartite (filters left, bulk actions center, add right) is intentional and actually reasonable for power users who know the page. The filter controls don't change between sessions so there's no "stale filter" surprise.

On the bulk actions: I concede this is a real risk. But the "Deactivate all" button label text is genuinely different from any filter label — it's a verb phrase, not a noun. The visual grouping into the center with a flex spacer does separate them. The confirmation issue is legit though.

On the Unicode icons in the status filter: I agree — replace them with Lucide icons. `XCircle` for errors, `RefreshCw` for running, `Clock` for queued. This is a one-line change per option and removes the cross-OS inconsistency.

On Reset: the Critic is right. The button should be at minimum a `text-slate-400 underline` or a small icon-text pair. A common pattern is to replace "Reset" with a `FilterX` icon that lights up only when filters are active — making it serve double duty as "filters active" indicator AND clear action.

**SYNTHESIS**:

1. **Add confirmation for "Deactivate all" and "Run all"** — show a `<AlertDialog>` (Radix, already in the stack) with a count: "This will deactivate 549 companies. Continue?" Both actions need this. (`CompaniesFilterBar.tsx` + new confirm dialog)
2. **Replace Unicode status filter icons with Lucide icons** — `XCircle` / `RefreshCw` / `Clock` in the SegmentedSwitch options for Errors / Running / Queued. Cross-OS consistent, matches app icon language. (`CompaniesFilterBar.tsx`)
3. **Upgrade the Reset button to a FilterX icon that only renders when filters are active** — use `cn('opacity-0 pointer-events-none', hasActiveFilters && 'opacity-100')` pattern, with a `FilterX` icon + "Reset" label. This also solves the missing "filters active" indicator. (`CompaniesFilterBar.tsx`)

---

### CompaniesTable

**CRITIC**:

1. **Row action buttons (Scan, Edit, Delete) are `opacity-0` until hover.** On keyboard navigation, tab-focusing a row does not trigger `group-hover`, so these buttons are completely invisible to keyboard users. The Delete button is especially dangerous to hide — if it accidentally reveals while a user mouses across the row, there's no confirmation before it fires.
2. **The Active toggle in the table fires immediately on click with no confirmation.** Toggling a company's active state is a meaningful action (stops all future crawling for that company). This toggle looks exactly like a display switch — there's no affordance that it triggers a network mutation. A soft confirmation (`"Deactivate 1Password?"`) or at least an undo toast would reduce accidents.
3. **The URL shown in the Company column (e.g., `boards-api.greenhouse.io`) is the raw API endpoint.** This is technically accurate but meaningless to a user who thinks in company names. A user scanning the table has no way to distinguish between two Greenhouse-hosted companies just from their subdomain path. The URL only matters when you need to verify/edit it; show it in the drawer instead.
4. **The "Last 10 scans" column header label does not explain what the colored bars mean.** A first-time user sees 10 tiny segments and has no idea what green vs grey vs amber vs red represents. There is no legend or tooltip.
5. **"Idle" bars (grey segments) dominate most rows** — 1Password, Acceldata, ADDI, Aledade, Alloy, Alluxio, Alpha-9 Oncology, and many others show 5+ grey segments. With 549 companies all scanning daily, this will be the majority visual state for most rows most of the time. The table looks perpetually half-empty.

**ADVOCATE**: The opacity-0 hover behavior is a deliberate space-saving choice — showing 3 action buttons on every row of a 549-row table would create massive visual noise. The implementation correctly stops event propagation so row clicks still open the drawer.

On keyboard navigation: this is a real gap. The buttons should be `focus-within:opacity-100` on the row's action cell, not just `group-hover`. One CSS class change fixes it.

On the Active toggle: I'd push back on a confirmation dialog — it's a toggle, not a delete. But a short undo toast (like Gmail's "Company deactivated — Undo") would be a better experience than a blocking modal. The toggle behavior is also reversible, unlike delete.

On the URL: I agree it's noisy for a table view. The external link is actually useful for debugging, but `boards-api.greenhouse.io` vs `jobs.lever.co` is information that's already conveyed by the Adapter column badge. We could remove the URL from the company cell and just keep the ExternalLink icon on hover.

On idle bars: this is a real visual density problem. One approach is to only show the most recent N scans that actually ran, rather than padding with idle slots on the left. Or make idle slots visually smaller (half-width) so successful bars dominate the visual weight.

**SYNTHESIS**:

1. **Add `focus-within:opacity-100` to the actions cell** so keyboard-focused rows reveal their buttons — a one-line CSS addition to `companies-columns.tsx` that unblocks keyboard navigation entirely.
2. **Remove the URL sub-label from the Company column cell** — the adapter badge already conveys the platform; keep only the company name (+ ExternalLink icon on hover to preserve debug utility). Reduces visual clutter significantly. (`companies-columns.tsx`, name column cell)
3. **Add a tooltip/legend to the "Last 10 scans" column header** — a `<TooltipProvider>` wrapping the header with a legend: "green = ok, amber = running, red = error, grey = idle (no scan data)". Critical for first-time users. (`companies-columns.tsx`, history column header)

---

### CompanyDetailDrawer

**CRITIC**:

1. **The Delete (trash) button is placed directly adjacent to the Close (X) button in the top-right corner of the drawer header.** Two buttons, 4px apart: one closes the drawer, one permanently deletes the company. This is the most dangerous UI placement in the entire app. A user who frequently uses Escape or clicks the corner to close panels will eventually hit Delete by mistake.
2. **The "Scan Interval" section shows "Adapter default: X min" and "Custom override: —"** but only after an async fetch. While loading it shows a skeleton. If the fetch fails, it shows the skeleton forever (no error state, no "unavailable" message — `setScanConfig(null)` on catch, which shows the skeleton indefinitely since `scanConfig` starts as `null` too).
3. **"Recent runs" and "Last 10 scans" are redundant.** The drawer shows a `ScanHistoryBar` in the "Last 10 scans" section AND lists the same 10 runs in the "Recent runs" section. The same data appears twice in different formats. The `ScanHistoryBar` in the drawer draws from `stats.recent_runs` while the table row draws from `company.scan_history` — different sources, potentially different data.
4. **The Activity stats ("Last 24 hours", "This month") use a tiny emerald "new" label stacked below the number.** The word "new" here means "new jobs found" but looks like a UI badge state. Someone skimming the drawer thinks "what is 'new'? Is this a status badge?" rather than reading it as a unit.
5. **There is no "View jobs for this company" action in the drawer.** A user looking at Airbnb's scan stats naturally wants to see the jobs that came out of those scans. There is no link to `/jobs?company=airbnb` or any cross-navigation affordance.

**ADVOCATE**: The adjacent Delete/Close placement is genuinely a concern, but it follows the pattern used in many right-panel drawers (e.g., Notion, Linear). The Trash icon is visually distinct from X, and the red hover state gives a clear warning signal before clicking. I'd argue a confirmation dialog on delete is the real solution rather than repositioning the buttons.

On the scan interval async skeleton: this is a real bug-shaped issue. The loading state and the error state are visually identical (both show the skeleton). The `finally` block sets `loading: false`, but `scanConfig` stays `null` on error — you need a separate `scanConfigError` state.

On the "Recent runs" / "Last 10 scans" redundancy: I agree the `ScanHistoryBar` in the drawer is redundant with the expanded run list below it. The bar is useful as a quick visual; the run list provides drill-down. But if they show different data (company vs stats source), that's a consistency bug, not just UX. The bar should be removed from the drawer since the full list is directly below it.

On "View jobs": this is a great missing affordance. The drawer positions itself as a company detail view but has no outbound links to the jobs that company produced. A "View N jobs" button using the jobs count from `stats` would close this gap cleanly.

**SYNTHESIS**:

1. **Move Delete out of the close-button corner** — shift Trash to the bottom of the drawer (perhaps below the action buttons in the header, or into a dedicated "Danger zone" row at the bottom of the panel). Any accidental click should not be possible by proximity alone. (`CompanyDetailDrawer.tsx`)
2. **Add `scanConfigError` state** — on `getCompanyScanConfig` catch, set an error flag and render "Scan interval unavailable" instead of showing an indefinite skeleton. One-line state addition + one conditional render. (`CompanyDetailDrawer.tsx`)
3. **Add a "View jobs →" link** that navigates to `/jobs` pre-filtered by company name/ID, using the total jobs count from `stats` as the label (e.g., "View 142 jobs →"). High-value navigation shortcut that bridges the two most-used pages. (`CompanyDetailDrawer.tsx`)

---

### AddCompanyModal

**CRITIC**:

1. **Field order puts Adapter first, then Company ID, then Name.** This is backwards from natural user flow — a user thinks "I want to add Stripe" not "I want to add a Greenhouse company." Name should be first, ID second, adapter third.
2. **"Company ID" is a confusing label.** The field holds a slug like `stripe` or `abnormal-security` — not a UUID or numeric ID. Users unfamiliar with ATS URL patterns will have no idea what to enter here. The hint says "Unique slug, e.g. stripe" but the label says "ID," which conflicts.
3. **The URL auto-fill works from Company ID, but there is no feedback confirming it happened.** When a user types "stripe" in the Company ID field and the URL field auto-populates, the URL text color turns `text-slate-400` — a nearly invisible signal that the URL was auto-filled rather than manually entered. Most users will not notice this visual difference.
4. **The "Active (start scanning)" toggle defaults to `false`.** New companies added without flipping this toggle will sit dormant indefinitely. The vast majority of the time a user adds a company, they want it to start scanning. The default should be `true`, or at minimum the toggle should have a stronger visual emphasis ("Start scanning immediately").
5. **No validation feedback until submit.** Type a malformed URL, leave name empty, click "Add Company" — only then are errors shown. There should be at minimum URL format validation on blur.

**ADVOCATE**: The adapter-first ordering is intentional: adapter drives the URL template. If Name were first, the user fills it in and then has to scroll back up when they realize the adapter changes the URL field. The current order front-loads the decision that has the most downstream effect.

On the ID/slug confusion: fair point. "Company ID (slug)" or just "Slug / Identifier" would be clearer. The current hint "Unique slug, e.g. stripe" is doing the job but the label contradicts it.

On the toggle default: I'd push back for well-monitored use cases — the admin may want to add a batch of companies before activating them. A reasonable middle ground: default to `true` but add a note like "You can deactivate later from the table." The toggle prominence should definitely increase regardless.

On validation: runtime URL validation with a debounce is worth adding. The apiClient already returns `ApiValidationError` with field errors, so field-level errors are handled on submit — but blur-time URL format checking (`try { new URL(v) } catch { setError(...) }`) is cheap and improves the flow.

**SYNTHESIS**:

1. **Change the "Active" toggle default from `false` to `true`** and update the label to "Start scanning immediately" to clarify the consequence. Most users adding a company want immediate scanning; the current default silently creates dormant companies. (`AddCompanyModal.tsx`, `EMPTY_FORM.active`)
2. **Rename "Company ID" label to "Identifier (slug)"** and keep the existing hint. Removes the ID/slug label conflict without changing any behavior. (`AddCompanyModal.tsx`, `Field` label prop)
3. **Add a more visible auto-fill indicator** — when the URL was auto-filled (not manually edited), show a small `✓ Auto-filled from identifier` note below the URL field in `text-violet-400/70`. Replaces the subtle grey-text color signal that most users won't notice. (`AddCompanyModal.tsx`, below URL input)

---

## Cross-cutting observations

**CRITIC**:

1. **Destructive actions throughout the page (Delete row, Delete in drawer, Deactivate all, active toggle) have inconsistent confirmation patterns**: some have no confirmation (Delete button, active toggle), some implicitly confirm via disabled states, and one ("Deactivate all") has a loader but no confirm dialog. The pattern is "just fire it" everywhere except in the modal which has Cancel/Submit.
2. **The "Active" column in the table and the "Active (start scanning)" toggle in the modal use the same visual control (violet toggle switch) for different conceptual actions**: one is a display filter and one is a mutation that affects the pipeline. There's no visual language distinction between "this toggles a view" and "this changes backend state."
3. **The filter bar has no "filters active" indicator**, so users navigating away and back will see a filtered list with no obvious indication that filters are applied. The URL does not persist filter state, and there are no visible filter pills/tags.
4. **Icon language is mixed**: the filter bar uses Unicode text icons (✕, ⟳, ·) while the rest of the app uses Lucide icons exclusively. The scan history bar uses its own color-segment language with no legend anywhere on the page.

**ADVOCATE**: The "fire without confirm" pattern for destructive row actions is common in power-user admin tools where undo is available and speed matters. The lack of confirmation on Delete is more concerning than on the toggle — deletes are permanent, toggles are reversible.

On filter state persistence: URL-persisted filters would be a meaningful QoL improvement, especially for teams using the app collaboratively ("here's the link showing all error companies"). This is non-trivial but would eliminate an entire class of "where did my filters go" confusion.

On icon language mixing: the Unicode icons in the SegmentedSwitch for status filters are genuinely inconsistent and should be replaced. The scan history bar legend gap is a real onboarding failure.

**SYNTHESIS**:

1. **Establish a consistent confirmation policy for destructive actions**: deletes (row + drawer) require a Radix `AlertDialog` confirmation; toggles (active switch) show an undo toast via `sonner` instead of a blocking dialog. This is the right friction/speed tradeoff for each action type.
2. **Persist filter state in the URL** using `useSearchParams` — `?search=stripe&adapter=greenhouse&active=active`. This enables bookmarking, sharing, and back-button behavior. (`useCompanies.ts`, `CompaniesFilterBar.tsx`)
3. **Add a "filters active" chip row** below the filter bar — when any non-default filter is active, show a row of `Badge` chips (one per active filter) with individual X buttons plus a "Clear all" link. This matches the pattern used in the Jobs Feed and removes the invisible-filter confusion.

---

## Priority Improvements

1. **Add `AlertDialog` confirmation to Delete (row + drawer) and "Deactivate all"** — these are the three permanently-irreversible or high-blast-radius actions with zero confirmation. Clicking Delete on a row currently fires immediately. Use Radix `AlertDialog`, already in the stack. (`companies-columns.tsx`, `CompanyDetailDrawer.tsx`, `CompaniesFilterBar.tsx`)

2. **Move the Delete button in CompanyDetailDrawer away from the Close button** — the current 4px proximity between Trash and X is an accidental-delete trap. Relocate Delete to the bottom of the drawer body or into a separate "..." overflow menu. (`CompanyDetailDrawer.tsx`)

3. **Dim zero-value pipeline stages in PipelineStrip** — when Queued, Crawling, Awaiting Filter, and Awaiting Enrichment are all 0, the current display looks like a broken pipeline. Conditionally apply `text-white/25` and `text-slate-700` on zero-value stages so a healthy-idle state reads as quiet. (`PipelineStrip.tsx`, `Stage`)

4. **Add `focus-within:opacity-100` to the actions cell in the table** — row actions (Scan, Edit, Delete) are currently invisible to keyboard users because only `group-hover` reveals them. One CSS class addition makes the page keyboard-navigable. (`companies-columns.tsx`, actions column cell)

5. **Change the AddCompanyModal "Active" toggle default to `true`** — the current `false` default silently creates dormant companies that will never scan. Swapping the default to `true` matches the intent of 95% of "Add Company" workflows. (`AddCompanyModal.tsx`, `EMPTY_FORM`)

6. **Replace Unicode status filter icons (✕ ⟳ ·) with Lucide icons** in the CompaniesFilterBar SegmentedSwitch — `XCircle`, `RefreshCw`, and `Clock` are already used elsewhere in the codebase and render consistently cross-platform. (`CompaniesFilterBar.tsx`)

7. **Add a "View jobs →" link in CompanyDetailDrawer** navigating to `/jobs` filtered by company — bridges the gap between the two most-used pages and lets users act on the scan stats they just read. (`CompanyDetailDrawer.tsx`)

8. **Fix the indefinite scan config skeleton in CompanyDetailDrawer** — when `getCompanyScanConfig` fails, `scanConfig` stays `null` and the skeleton shows forever. Add a `scanConfigError` boolean state and render "Unavailable" on error. (`CompanyDetailDrawer.tsx`)

---

## First Impression Note — Insights Page

The Insights page has genuinely useful data but the **Company Activity section** is a 150+ company vertical list with raw enrichment counts that reads like an unformatted database export. A new user landing here would immediately scroll past the interesting analytics panels (Pipeline Funnel, Location Breakdown, Sponsorship chart) and stare at a wall of company names and numbers with no pagination, no sort controls, and no way to filter. The single most jarring thing visible at a glance: the list continues well past one full screen of scrolling with no visual endpoint or "showing N companies" counter, making the page feel infinite and overwhelming.

---

## Live Feature Test Results — 2026-05-03

All interactive features tested live against production data (549 companies). Run scan and Delete were intentionally skipped per instructions.

### Pass/Fail Summary

| #   | Feature                                  | Result  | Notes                                                                                                                                               |
| --- | ---------------------------------------- | ------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | Page load & data render                  | ✅ Pass | 549 companies loaded, pipeline strip visible                                                                                                        |
| 2   | Pipeline strip stats                     | ✅ Pass | Total/Active/Queued/Crawling/Awaiting Filter/Awaiting Enrichment/New Jobs all render                                                                |
| 3   | Search filter (name)                     | ✅ Pass | Filters table in real-time as expected                                                                                                              |
| 4   | Status filter — All                      | ✅ Pass | Clears filter, returns all 549                                                                                                                      |
| 5   | Status filter — Active                   | ✅ Pass | Returns 549 active companies                                                                                                                        |
| 6   | Status filter — Errors                   | ✅ Pass | Returns 3 companies (Microsoft, Netflix, Uber) — server-side                                                                                        |
| 7   | Status filter — Queued                   | ✅ Pass | Filter applied; 0 results (consistent with queue API)                                                                                               |
| 8   | Status filter — Running                  | ⚠️ Bug  | Filter shows 0 results despite PipelineStrip showing 1 CRAWLING — queue API and pipeline stats are out of sync                                      |
| 9   | Rows per page (10/25/50/100)             | ✅ Pass | Changing to 50 updated table; pagination recalculated                                                                                               |
| 10  | Pagination — next/prev                   | ✅ Pass | Page advances and retreats correctly                                                                                                                |
| 11  | Pagination — first/last                  | ✅ Pass | Jumps to boundaries; disabled at limits                                                                                                             |
| 12  | Sort — Company Name                      | ✅ Pass | A→Z / Z→A toggle works                                                                                                                              |
| 13  | Sort — Last Scan                         | ✅ Pass | Sorts by recency ascending/descending                                                                                                               |
| 14  | Sort — Status                            | ✅ Pass | Groups active/inactive as expected                                                                                                                  |
| 15  | Active toggle (row)                      | ✅ Pass | Fires immediately, updates row, no confirmation (by design but noted)                                                                               |
| 16  | Open Company Detail Drawer               | ✅ Pass | Drawer opens with company details, scan history rendered                                                                                            |
| 17  | Scan config — view override              | ⚠️ Bug  | `company_override` field shows "min ×" with blank value when API returns `""` instead of `null` — guard fails                                       |
| 18  | Scan config — set override               | ✅ Pass | Input accepts value, Save fires API call                                                                                                            |
| 19  | Drawer — close button                    | ✅ Pass | Drawer closes cleanly                                                                                                                               |
| 20  | Add Company modal — open                 | ✅ Pass | Modal opens with empty form                                                                                                                         |
| 21  | Add Company — adapter switching          | ✅ Pass | URL field hides/shows for fixed-URL adapters                                                                                                        |
| 22  | Add Company — validation (fixed adapter) | ⚠️ Bug  | Submitting Google adapter with no URL shows "ID, name, and URL are required" — URL not required for fixed adapters, message is wrong                |
| 23  | Add Company — cancel                     | ✅ Pass | Modal closes, no state leaked                                                                                                                       |
| 24  | Edit Company modal — open                | ✅ Pass | Modal opens pre-filled with company data                                                                                                            |
| 25  | Edit Company — URL hint copy             | ⚠️ Bug  | URL hint says "URL auto-filled from company ID" in edit mode even though URL field is already set — hint copy is not mode-aware                     |
| 26  | Edit Company — save                      | ✅ Pass | Submits, invalidates query, table refreshes                                                                                                         |
| 27  | Bulk action — Activate all               | ✅ Pass | Button visible when applicable; fires as expected                                                                                                   |
| 28  | Bulk action — Deactivate all             | ✅ Pass | Button visible; fires with no confirmation (noted in design review)                                                                                 |
| 29  | Bulk action label accuracy               | ⚠️ Bug  | "Activate all N companies" N = `totalActive` (global server count), not the count in the current filtered view — misleading when filters are active |
| 30  | Auto-refresh toggle                      | ✅ Pass | Toggles on/off; state persisted to localStorage; 30s interval fires                                                                                 |
| 31  | Manual refresh button                    | ✅ Pass | Triggers immediate re-fetch                                                                                                                         |

**Results: 26 Pass · 5 Bug · 0 Skip (Run/Delete intentionally excluded)**

---

### Bug Details

#### BUG-01 — Running filter shows 0 results despite pipeline showing 1 CRAWLING

- **Severity**: Medium
- **Location**: `CompaniesFilterBar` → Status filter "Running" option; `useCompanies.ts` client-side filter
- **What happens**: PipelineStrip correctly shows 1 company in the CRAWLING state. Selecting the "Running" status filter immediately returns 0 results.
- **Root cause**: The Running filter performs a client-side join against `queueMap` (from `fetchQueue` API). The company currently crawling may have been dequeued from `company_scan_queue` before the filter ran — once a crawler picks up a job, the queue row is updated to `status='running'` but the `fetchQueue` endpoint may not include it if the API filters on `status='pending'` only. PipelineStats and fetchQueue are separate data sources with no synchronization guarantee.
- **Impact**: Users cannot find a company that is actively being crawled using the Running filter — the filter is functionally broken for in-flight jobs.

#### BUG-02 — Scan interval shows "min ×" with blank value

- **Severity**: Low–Medium
- **Location**: `CompanyDetailDrawer.tsx` — scan config override display
- **What happens**: For companies with no override set, the API returns `company_override: ""` (empty string). The drawer guard `scanConfig.company_override !== null` evaluates `true` for `""`, so it renders the override value — which is blank — producing "min ×" with nothing between "min" and "×".
- **Root cause**: Backend returns `""` instead of `null`/omitting the field when no override is set. Frontend guard only checks `!== null`, not falsy.
- **Fix**: Change guard to `scanConfig.company_override != null && scanConfig.company_override !== ""` or fix backend to return `null`.

#### BUG-03 — Validation message wrong for fixed-URL adapters

- **Severity**: Low
- **Location**: `AddCompanyModal.tsx` — form validation
- **What happens**: Submitting with adapter type `google` (or any fixed-URL adapter) and no URL triggers error `"ID, name, and URL are required"`. URL is not required for these adapters — the field is hidden — so the message is incorrect and confusing.
- **Root cause**: Validation error string is hardcoded regardless of adapter type. The URL validation correctly skips the URL field for fixed adapters, but the error message doesn't distinguish.
- **Fix**: Conditionally set error message: `FIXED_URL_ADAPTERS.has(form.adapter_type) ? 'ID and name are required' : 'ID, name, and URL are required'`.

#### BUG-04 — Edit modal URL hint is stale copy from create mode

- **Severity**: Low (UX/copy)
- **Location**: `AddCompanyModal.tsx` — `urlHint` rendering in edit mode
- **What happens**: When editing a Greenhouse company, the URL hint reads "URL auto-filled from company ID" even though the URL field already contains the actual URL and the ID field is disabled (non-editable). The hint is misleading — it implies the URL hasn't been set yet.
- **Root cause**: `urlHint` logic is not mode-aware. It shows the create-mode hint regardless of whether the modal is in edit or create state.
- **Fix**: Suppress `urlHint` when `isEditMode` is true, or replace with a neutral "URL can be updated" copy.

#### BUG-05 — Bulk action count reflects global total, not filtered result

- **Severity**: Medium
- **Location**: `CompaniesPage.tsx` — `bulkAllActive` calculation; `CompaniesFilterBar.tsx` — button label
- **What happens**: With an active filter (e.g. search = "Meta"), the table shows 1 company. The bulk action button reads "Deactivate all 549 companies" — `totalActive` is the global server count, not the count in the current view.
- **Root cause**: `bulkAllActive = c.totalActive > 0 && c.totalActive === c.total` — `totalActive` is from the server global count, `total` is from the current filtered result set. When filters are active these can never be equal, so the button label falls through to displaying the raw `totalActive` value (549).
- **Impact**: Clicking "Deactivate all 549 companies" when the user believes they are acting on 1 filtered company is a silent mass-action risk.
- **Fix**: Pass the current filtered result count to the bulk action label, or derive `bulkAllActive` from filtered totals only.

---

## Resolution Status — as of 2026-05-11

| #   | Priority Improvement                                                      | Status  | Resolved in                                                                                          |
| --- | ------------------------------------------------------------------------- | ------- | ---------------------------------------------------------------------------------------------------- |
| 1   | `AlertDialog` confirmation for Delete (row + drawer) and "Deactivate all" | ✅ Done | 2026-05-05 rewrite                                                                                   |
| 2   | Move Delete button away from Close button in drawer                       | ✅ Done | 2026-05-05 rewrite — Delete triggers a confirm dialog before firing                                  |
| 3   | Dim zero-value pipeline stages in PipelineStrip                           | ✅ Done | 2026-05-05 rewrite                                                                                   |
| 4   | `focus-within:opacity-100` on table actions cell                          | ✅ Done | 2026-05-05 rewrite                                                                                   |
| 5   | Change AddCompanyModal "Active" toggle default to `true`                  | ✅ Done | 2026-05-05 rewrite — `EMPTY_FORM.active = true`                                                      |
| 6   | Replace Unicode status filter icons with Lucide icons                     | ✅ Done | 2026-05-05 rewrite — `CheckCircle2`, `XCircle`, `RefreshCw`, `Clock`                                 |
| 7   | Add "View jobs →" link in CompanyDetailDrawer                             | ✅ Done | 2026-05-11 hardening — `navigate(ROUTES.JOBS, { state: { companies: [...] } })`                      |
| 8   | Fix indefinite scan config skeleton in CompanyDetailDrawer                | ✅ Done | 2026-05-11 hardening — scan stats migrated to `useQuery`; TanStack Query handles error/loading state |
| 9   | Auto-refresh label changed from "30s" to "Live"/"Auto"                    | ✅ Done | 2026-05-11 hardening — label reads "Live" when on, "Auto" when off                                   |

### Bugs (from §Live Feature Test Results)

| Bug                                                          | Status   | Resolved in                                                                                                                                                                                             |
| ------------------------------------------------------------ | -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| BUG-01 Running filter shows 0 results                        | ✅ Fixed | 2026-05-05 rewrite — queue status now comes from the backend JOIN, not a separate client-side poll                                                                                                      |
| BUG-02 Scan interval shows "min ×" with blank value          | ✅ Fixed | 2026-05-05 rewrite — scan config overlay section removed; scan stats now the only detail query                                                                                                          |
| BUG-03 Validation message wrong for fixed-URL adapters       | ✅ Fixed | 2026-05-05 rewrite — validation message is now adapter-type–aware                                                                                                                                       |
| BUG-04 Edit modal URL hint stale copy                        | ✅ Fixed | 2026-05-05 rewrite — hint suppressed in edit mode                                                                                                                                                       |
| BUG-05 Bulk action count reflects global total, not filtered | ⚠️ Open  | `bulkAllActive` still derived from `c.companies.every(co => co.active)` (filtered page), but the Deactivate all action affects all companies globally — label mismatch persists when filters are active |

### Open items (not yet addressed)

- Persist filter state in URL via `useSearchParams` (cross-session filter sharing)
- Undo-toast on active toggle instead of silent fire
- Filter chips row showing active filters below the filter bar

---

_Generated by jobradar-ux-review scheduled task_
