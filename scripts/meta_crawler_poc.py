#!/usr/bin/env python3
"""End-to-end Meta Careers detail-page extraction — Python proof of concept.

Validates that Meta's job detail pages can be scraped over plain HTTP by
parsing the inline <script type="application/ld+json"> block, which contains
a schema.org JobPosting payload with title, description, responsibilities,
qualifications, employmentType, datePosted, validThrough, jobLocation, and
hiringOrganization.

Listing extraction is NOT covered here — Meta's /jobsearch page is fully
client-rendered (React + GraphQL with rotating CSRF tokens) and an HTTP-only
listing crawler is a separate, larger project. This PoC focuses on the
detail-page extractor that will replace the rod-based fetchDetailPage.

Usage:
    # Hits real Meta:
    python3 meta_crawler_poc.py --job-id 1187958502273564

    # Multiple jobs with production-style pacing:
    python3 meta_crawler_poc.py --job-id 1187958502273564 --job-id 9870 --sleep

    # Default test ID if none given:
    python3 meta_crawler_poc.py
"""
from __future__ import annotations

import argparse
import json
import os
import random
import re
import ssl
import sys
import time
import urllib.error
import urllib.request

# ── Constants ──────────────────────────────────────────────────────────────
# Canonical detail URL — Meta's `/jobs/{id}/` redirects here, but urllib gets
# a 400 on the redirect-via-shortcut form, so we hit the canonical path
# directly. Verified via browser: 200 OK, no redirect, full JSON-LD payload.
DETAIL_BASE = "https://www.metacareers.com/profile/job_details/"

# Listing URL — pre-filtered to Full-time roles in the user's target offices.
# NOT used by this POC (Meta's listing is fully client-rendered via GraphQL,
# so an HTTP GET on this URL returns an empty SPA shell). Stored here for
# reference / future GraphQL or rod-listing work.
LISTING_URL = (
    "https://www.metacareers.com/jobsearch"
    "?roles[0]=Full%20time%20employment"
    "&offices[0]=Menlo%20Park%2C%20CA"
    "&offices[1]=Irvine%2C%20CA"
    "&offices[2]=Newark%2C%20CA"
    "&offices[3]=Fremont%2C%20CA"
    "&offices[4]=Pasadena%2C%20CA"
    "&offices[5]=San%20Diego%2C%20CA"
    "&offices[6]=San%20Mateo%2C%20CA"
    "&offices[7]=Sausalito%2C%20CA"
    "&offices[8]=Sunnyvale%2C%20CA"
    "&offices[9]=Burlingame%2C%20CA"
    "&offices[10]=Foster%20City%2C%20CA"
    "&offices[11]=Northridge%2C%20CA"
    "&offices[12]=Los%20Angeles%2C%20CA"
    "&offices[13]=Santa%20Clara%2C%20CA"
    "&offices[14]=San%20Francisco%2C%20CA"
    "&offices[15]=Mountain%20View%2C%20CA"
    "&offices[16]=Seattle%2C%20WA"
    "&offices[17]=New%20York%2C%20NY"
    "&offices[18]=Austin%2C%20TX"
    "&offices[19]=Temple%2C%20TX"
    "&offices[20]=Garland%2C%20TX"
    "&offices[21]=Houston%2C%20TX"
    "&offices[22]=El%20Paso%2C%20TX"
    "&offices[23]=Fort%20Worth%2C%20TX"
)

UA = (
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
    "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36"
)
ACCEPT = (
    "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8"
)
ACCEPT_LANG = "en-US,en;q=0.9"

# Full Chrome header set Meta seems to validate against. urllib's default
# 3-header request gets a 400; with these it parses cleanly.
BROWSER_HEADERS = {
    "User-Agent": UA,
    "Accept": ACCEPT,
    "Accept-Language": ACCEPT_LANG,
    "sec-ch-ua": '"Chromium";v="121", "Not A(Brand";v="99", "Google Chrome";v="121"',
    "sec-ch-ua-mobile": "?0",
    "sec-ch-ua-platform": '"macOS"',
    "Sec-Fetch-Dest": "document",
    "Sec-Fetch-Mode": "navigate",
    "Sec-Fetch-Site": "none",
    "Sec-Fetch-User": "?1",
    "Upgrade-Insecure-Requests": "1",
}

# Detail pacing — production target is 3-7 s between detail fetches.
DETAIL_MIN_S = 3
DETAIL_MAX_S = 7

# A known-good test ID (Software Engineer, Technical Leadership). Used if
# the caller doesn't pass --job-id.
DEFAULT_JOB_ID = "1187958502273564"

# A diverse sample of real Meta job IDs (different roles, locations, teams)
# scraped from the listing page. Use --batch to run all of them and get a
# coverage report — confirms the parser handles every shape Meta emits.
SAMPLE_JOB_IDS = [
    "1187958502273564",  # Software Engineer (Technical Leadership)
    "7868993783158276",  # Production Engineering
    "4989773394582242",  # NPI Operations Program Manager
    "1436181490732782",  # Software Engineer, Machine Learning
    "2769420660122910",  # MSL Product Designer
    "677160418622314",   # Software Engineer, Infrastructure
    "998357492128826",   # Software Engineer, Machine Learning
    "1420853345578936",  # Product Manager
    "2486789151677735",  # Software Engineer, Infrastructure
    "1570455808415085",  # Principal, Energy Infrastructure Investments
    "691715269835265",   # Product Manager, Machine Learning
]

# ── Regex / extraction ─────────────────────────────────────────────────────
RE_JSON_LD = re.compile(
    r'<script\s+type=["\']application/ld\+json["\'][^>]*>(.*?)</script>',
    re.IGNORECASE | re.DOTALL,
)
RE_TAG = re.compile(r"<[^>]+>")
RE_WS = re.compile(r"\s+")


# ── HTTP helper ────────────────────────────────────────────────────────────
def _build_ssl_context(insecure: bool) -> ssl.SSLContext:
    if insecure:
        ctx = ssl.create_default_context()
        ctx.check_hostname = False
        ctx.verify_mode = ssl.CERT_NONE
        print("  [warn] TLS verification DISABLED (--insecure)", flush=True)
        return ctx
    try:
        import certifi  # type: ignore
        return ssl.create_default_context(cafile=certifi.where())
    except ImportError:
        return ssl.create_default_context()


def fetch(url: str, ssl_ctx: ssl.SSLContext, timeout: int = 30) -> str:
    req = urllib.request.Request(url, headers=BROWSER_HEADERS)
    try:
        with urllib.request.urlopen(req, timeout=timeout, context=ssl_ctx) as r:
            return r.read().decode("utf-8", errors="replace")
    except urllib.error.HTTPError as e:
        # Surface Meta's actual error body so we can diagnose 400s cleanly.
        body = ""
        try:
            body = e.read().decode("utf-8", errors="replace")
        except Exception:
            pass
        print(f"  [http {e.code}] {e.reason}", flush=True)
        if body:
            print(f"  [body 200 chars] {body[:200]}", flush=True)
        raise


def sleep_interval(label: str) -> None:
    d = random.uniform(DETAIL_MIN_S, DETAIL_MAX_S)
    print(f"  [pace] sleep {d:.1f}s ({label})", flush=True)
    time.sleep(d)


# ── JSON-LD extraction ─────────────────────────────────────────────────────
def find_job_posting(html: str) -> dict | None:
    """Find the first <script type="application/ld+json"> block whose JSON
    payload has @type == JobPosting and return the parsed dict."""
    for m in RE_JSON_LD.finditer(html):
        raw = m.group(1).strip()
        try:
            data = json.loads(raw)
        except json.JSONDecodeError:
            continue
        candidates = []
        if isinstance(data, dict):
            candidates.append(data)
            if "@graph" in data and isinstance(data["@graph"], list):
                candidates.extend([n for n in data["@graph"] if isinstance(n, dict)])
        elif isinstance(data, list):
            candidates.extend([n for n in data if isinstance(n, dict)])
        for c in candidates:
            if c.get("@type") == "JobPosting":
                return c
    return None


def html_to_text(s: str) -> str:
    """Strip HTML tags and collapse whitespace. Meta's `description`,
    `responsibilities` and `qualifications` fields are HTML-flavoured strings
    with <br>, &nbsp;, etc."""
    if not s:
        return ""
    text = RE_TAG.sub(" ", s)
    text = text.replace("&nbsp;", " ").replace("&amp;", "&")
    text = text.replace("&lt;", "<").replace("&gt;", ">").replace("&quot;", '"')
    text = RE_WS.sub(" ", text).strip()
    return text


def _addr_str(value) -> str:
    """Schema.org address parts come in many flavours in Meta's payloads:
    - plain strings ("USA")
    - nested objects ({"@type":"Country","name":"USA"})
    - arrays (one entry per parallel jobLocation)
    - and combinations: dicts whose "name" is itself a list, lists of dicts,
      etc. We recurse on every container until we land on plain strings,
      then dedupe."""
    if not value:
        return ""
    if isinstance(value, str):
        return value
    if isinstance(value, dict):
        # Recurse into the standard schema.org name field rather than str()-ing
        # it directly, because Meta sometimes puts a list there.
        return _addr_str(value.get("name"))
    if isinstance(value, list):
        seen, parts = set(), []
        for v in value:
            s = _addr_str(v)
            if s and s not in seen:
                seen.add(s)
                parts.append(s)
        return ", ".join(parts)
    return ""


def flatten_locations(loc) -> list[str]:
    """Meta's jobLocation is either a single object or an array of them.
    Each has .address.{addressLocality, addressRegion, addressCountry}.
    Dedupe the final list — multiple entries often share the same city when
    Meta encodes per-building addresses for a single office."""
    if not loc:
        return []
    if isinstance(loc, dict):
        loc = [loc]
    seen, out = set(), []
    for entry in loc:
        if not isinstance(entry, dict):
            continue
        addr = entry.get("address") or {}
        parts = [
            _addr_str(addr.get("addressLocality")),
            _addr_str(addr.get("addressRegion")),
            _addr_str(addr.get("addressCountry")),
        ]
        joined = ", ".join([p for p in parts if p])
        if joined and joined not in seen:
            seen.add(joined)
            out.append(joined)
    return out


def build_description(j: dict) -> str:
    """Compose the same kind of multi-section description string the Google
    adapter emits, using every signal Meta's JSON-LD exposes."""
    title = (j.get("title") or "").strip()
    locations = flatten_locations(j.get("jobLocation"))
    location_line = "; ".join(locations[:5])
    if len(locations) > 5:
        location_line += f" (+{len(locations) - 5} more)"

    lines = [title, "", location_line, ""]

    # Metadata block — keep these one-line so they don't muddy LLM input later.
    meta_pairs = []
    if (org := (j.get("hiringOrganization") or {}).get("name")):
        meta_pairs.append(("Organization", str(org)))
    if (et := j.get("employmentType")):
        meta_pairs.append(("Employment type", str(et)))
    if (ic := j.get("industry")):
        meta_pairs.append(("Industry", _addr_str(ic)))
    if (oc := j.get("occupationalCategory")):
        meta_pairs.append(("Occupational category", _addr_str(oc)))
    if (posted := j.get("datePosted")):
        meta_pairs.append(("Posted", str(posted)))
    if (valid := j.get("validThrough")):
        meta_pairs.append(("Valid through", str(valid)))
    if (apply := j.get("directApply")) is not None:
        meta_pairs.append(("Direct apply", str(apply)))
    if meta_pairs:
        for k, v in meta_pairs:
            if v:
                lines.append(f"{k}: {v}")
        lines.append("")

    # Salary — present on some Meta postings, absent on others.
    base = j.get("baseSalary") or {}
    sval = base.get("value") if isinstance(base, dict) else None
    if isinstance(sval, dict):
        lo, hi = sval.get("minValue"), sval.get("maxValue")
        unit = sval.get("unitText") or ""
        cur = base.get("currency") or ""
        if lo or hi:
            r = f"{lo}-{hi}" if (lo and hi) else f"{lo or hi}"
            lines.append(f"Compensation: {r} {cur} {unit}".strip())
            lines.append("")

    desc = html_to_text(j.get("description"))
    resp = html_to_text(j.get("responsibilities"))
    qual = html_to_text(j.get("qualifications"))

    if desc:
        lines.append("Description")
        lines.append(desc)
        lines.append("")
    if resp:
        lines.append("Responsibilities")
        lines.append(resp)
        lines.append("")
    if qual:
        lines.append("Qualifications")
        lines.append(qual)
        lines.append("")

    return "\n".join(lines).strip()


def all_fields_summary(j: dict) -> list[tuple[str, str]]:
    """Enumerate every top-level key in the JobPosting JSON-LD with a
    one-line preview. Useful for the Go port: shows every signal Meta
    actually emits in production, so we know what to model in the struct."""
    out: list[tuple[str, str]] = []
    for k in sorted(j.keys()):
        v = j[k]
        if v is None:
            continue
        if isinstance(v, (str, int, float, bool)):
            preview = str(v)
        elif isinstance(v, dict):
            preview = "{" + ", ".join(sorted(v.keys())[:8]) + "}"
        elif isinstance(v, list):
            if not v:
                preview = "[]"
            else:
                first = v[0]
                if isinstance(first, dict):
                    preview = f"[{len(v)}] of {{" + ", ".join(sorted(first.keys())[:6]) + "}"
                else:
                    preview = f"[{len(v)}] e.g. {first!r}"
        else:
            preview = type(v).__name__
        if len(preview) > 120:
            preview = preview[:117] + "..."
        out.append((k, preview))
    return out


# ── Batch coverage report (across many jobs) ──────────────────────────────
# Tracked field names — anything missing from a posting shows up as "absent"
# in the coverage table at the end of a batch run.
COVERAGE_FIELDS = [
    "title",
    "description",
    "responsibilities",
    "qualifications",
    "employmentType",
    "datePosted",
    "validThrough",
    "directApply",
    "hiringOrganization",
    "jobLocation",
    "industry",
    "occupationalCategory",
    "baseSalary",
    "skills",
    "jobBenefits",
]


def field_present(j: dict, key: str) -> bool:
    v = j.get(key)
    if v is None:
        return False
    if isinstance(v, str):
        return v.strip() != ""
    if isinstance(v, (list, dict)):
        return len(v) > 0
    return True


def run_batch(job_ids: list[str], insecure: bool, do_sleep: bool) -> int:
    """Run the parser across many jobs and print a coverage table.
    Catches edge cases the single-job test can't surface."""
    ssl_ctx = _build_ssl_context(insecure)
    rows = []  # one dict per job
    failed = []

    for i, job_id in enumerate(job_ids):
        if i > 0 and do_sleep:
            sleep_interval(f"between detail {i}/{len(job_ids)}")
        url = f"{DETAIL_BASE}{job_id}"
        try:
            html = fetch(url, ssl_ctx)
        except urllib.error.URLError as e:
            failed.append((job_id, str(e)))
            continue
        job = find_job_posting(html)
        if job is None:
            failed.append((job_id, "no JobPosting JSON-LD"))
            continue

        locations = flatten_locations(job.get("jobLocation"))
        rows.append({
            "id": job_id,
            "title": (job.get("title") or "")[:50],
            "loc_count": len(locations),
            "first_loc": locations[0] if locations else "",
            "desc_chars": len(html_to_text(job.get("description")) or ""),
            "resp_chars": len(html_to_text(job.get("responsibilities")) or ""),
            "qual_chars": len(html_to_text(job.get("qualifications")) or ""),
            "fields": {f: field_present(job, f) for f in COVERAGE_FIELDS},
            "all_keys": sorted(job.keys()),
            "size_kb": len(html) // 1024,
        })

    print(f"\n{'='*70}")
    print(f"BATCH SUMMARY  —  parsed: {len(rows)}/{len(job_ids)}, failed: {len(failed)}")
    print(f"{'='*70}\n")

    # Per-job summary line
    print(f"{'ID':<22} {'Title':<40} {'KB':>4} {'Locs':>4} {'Desc':>5} {'Resp':>5} {'Qual':>5}")
    print("-" * 95)
    for r in rows:
        print(
            f"{r['id']:<22} {r['title'][:40]:<40} "
            f"{r['size_kb']:>4} {r['loc_count']:>4} "
            f"{r['desc_chars']:>5} {r['resp_chars']:>5} {r['qual_chars']:>5}"
        )

    # Field coverage matrix — which fields are present per job
    print(f"\n{'='*70}")
    print("FIELD COVERAGE  —  '#' = present, '.' = missing")
    print(f"{'='*70}\n")
    print(f"{'Field':<25} " + " ".join(f"{r['id'][-4:]:>4}" for r in rows))
    for f in COVERAGE_FIELDS:
        marks = " ".join(f"{'#' if r['fields'][f] else '.':>4}" for r in rows)
        present = sum(1 for r in rows if r["fields"][f])
        print(f"{f:<25} {marks}   ({present}/{len(rows)})")

    # Any keys we didn't track but that show up in real data
    all_keys_seen = set()
    for r in rows:
        all_keys_seen.update(r["all_keys"])
    untracked = sorted(all_keys_seen - set(COVERAGE_FIELDS) - {"@context", "@type"})
    if untracked:
        print(f"\nOther keys observed (not in COVERAGE_FIELDS):")
        for k in untracked:
            count = sum(1 for r in rows if k in r["all_keys"])
            print(f"  {k:<25} ({count}/{len(rows)} jobs)")

    if failed:
        print(f"\nFailures:")
        for jid, err in failed:
            print(f"  {jid}: {err}")
    return 0 if not failed else 1


# ── End-to-end driver ──────────────────────────────────────────────────────
def run(job_ids: list[str], insecure: bool, do_sleep: bool) -> int:
    ssl_ctx = _build_ssl_context(insecure)

    for i, job_id in enumerate(job_ids):
        if i > 0 and do_sleep:
            sleep_interval(f"between detail {i}/{len(job_ids)}")

        url = f"{DETAIL_BASE}{job_id}"
        print(f"\n[detail] GET {url}", flush=True)
        try:
            html = fetch(url, ssl_ctx)
        except urllib.error.URLError as e:
            print(f"  [error] HTTP failed: {e}", file=sys.stderr)
            continue

        print(f"  size={len(html):,} bytes")

        job = find_job_posting(html)
        if job is None:
            print("  [error] No JobPosting JSON-LD block found")
            continue

        # Print the parsed fields (cap each at a sane preview length)
        print(f"  type            = {job.get('@type')}")
        print(f"  title           = {job.get('title')}")
        print(f"  org             = {(job.get('hiringOrganization') or {}).get('name')}")
        print(f"  employmentType  = {job.get('employmentType')}")
        print(f"  datePosted      = {job.get('datePosted')}")
        print(f"  validThrough    = {job.get('validThrough')}")
        print(f"  directApply     = {job.get('directApply')}")
        locations = flatten_locations(job.get("jobLocation"))
        print(f"  locations({len(locations)}):")
        for loc in locations:
            print(f"    - {loc}")
        print(f"  description     = {len(html_to_text(job.get('description')) or ''):,} chars")
        print(f"  responsibilities= {len(html_to_text(job.get('responsibilities')) or ''):,} chars")
        print(f"  qualifications  = {len(html_to_text(job.get('qualifications')) or ''):,} chars")

        # Full enumeration of every top-level JSON-LD key Meta actually emits
        # — this is the catalogue we use when shaping the Go struct later.
        print("\n  --- all JSON-LD top-level fields ---")
        for k, preview in all_fields_summary(job):
            print(f"  {k:24s} {preview}")
        print("  --- end fields ---")

        # Composed description (title + locations + 3 sections)
        desc = build_description(job)
        print(f"\n  --- composed description ({len(desc):,} chars) ---")
        for line in desc.splitlines():
            print(f"  {line}")
        print(f"  --- end ---")
    return 0


def main() -> int:
    p = argparse.ArgumentParser()
    p.add_argument(
        "--job-id",
        action="append",
        help="Meta job ID (numeric). Repeat for multiple. Defaults to one known test job.",
    )
    p.add_argument(
        "--sleep",
        action="store_true",
        help=f"Sleep {DETAIL_MIN_S}-{DETAIL_MAX_S}s between detail fetches",
    )
    p.add_argument(
        "--insecure",
        action="store_true",
        help="Disable TLS cert verification (macOS Python missing-CA workaround)",
    )
    p.add_argument(
        "--batch",
        action="store_true",
        help="Run across the SAMPLE_JOB_IDS list and print a coverage table",
    )
    args = p.parse_args()
    if args.batch:
        ids = args.job_id or SAMPLE_JOB_IDS
        return run_batch(ids, args.insecure, args.sleep)
    ids = args.job_id or [DEFAULT_JOB_ID]
    return run(ids, args.insecure, args.sleep)


if __name__ == "__main__":
    sys.exit(main())
