#!/usr/bin/env python3
"""
POC: discover ATS-hosted companies via free CDX indexes, then validate each
candidate slug against the ATS's own public job board API.

Supports Greenhouse, Ashby, and Lever (see ats_specs.py for the per-ATS
config). Extraction and validation are both plain deterministic functions -
no LLM anywhere. Zero recurring cost: every CDX index and every ATS API used
here is free and unauthenticated.

Usage:
    python3 discover.py --ats greenhouse [--max-slugs 300] [--num-indexes 3] [--delay 0.25]
    python3 discover.py --ats ashby
    python3 discover.py --ats lever
    python3 discover.py --ats all
"""

import argparse
import csv
import json
import re
import time
import urllib.parse
from datetime import datetime, timedelta, timezone
from pathlib import Path

import requests

from ats_specs import ATS_SPECS, CDX_SOURCE_COMMONCRAWL, CDX_SOURCE_WAYBACK, ATSSpec

CDX_COLLINFO_URL = "https://index.commoncrawl.org/collinfo.json"
COMMONCRAWL_QUERY_TEMPLATE = "https://index.commoncrawl.org/{index_id}-index"
WAYBACK_QUERY_URL = "https://web.archive.org/cdx/search/cdx"
WAYBACK_LOOKBACK_DAYS = 730  # only trust captures from the last ~2 years

# Wayback's CDX API returns results in alphabetical urlkey order with no
# resumeKey support on this endpoint - a single flat query over-samples
# slugs starting with digits/"a". Querying per starting character instead
# gives roughly even coverage across the slug namespace.
WAYBACK_SLUG_PREFIXES = list("0123456789abcdefghijklmnopqrstuvwxyz")

SLUG_DENYLIST = {"embed", "assets", "static", "api", "css", "js", "favicon.ico", "robots.txt"}
SLUG_PATTERN = re.compile(r"^[a-z0-9][a-z0-9._-]{0,63}$")

OUTPUT_DIR = Path(__file__).parent / "output"


def extract_slug(url: str, domain: str) -> str | None:
    parsed = urllib.parse.urlparse(url)
    if parsed.netloc.lower() != domain:
        return None
    path = urllib.parse.unquote(parsed.path)
    segment = path.strip("/").split("/", 1)[0].strip().lower()
    if not segment or segment in SLUG_DENYLIST:
        return None
    if not SLUG_PATTERN.match(segment):
        return None
    return segment


def parse_cdx_lines(text: str, domain: str) -> tuple[set[str], int]:
    slugs: set[str] = set()
    malformed = 0
    for line in text.splitlines():
        line = line.strip()
        if not line or line.startswith("["):  # wayback's leading header row
            continue
        try:
            record = json.loads(line)
        except json.JSONDecodeError:
            malformed += 1
            continue
        slug = extract_slug(record.get("url", ""), domain)
        if slug:
            slugs.add(slug)
    return slugs, malformed


def get_latest_commoncrawl_index_ids(num_indexes: int) -> list[str]:
    resp = requests.get(CDX_COLLINFO_URL, timeout=30)
    resp.raise_for_status()
    collections = resp.json()
    return [c["id"] for c in collections[:num_indexes]]


def fetch_commoncrawl_slugs(session: requests.Session, index_id: str, domain: str,
                             page_limit: int, max_retries: int = 3) -> set[str]:
    params = {"url": f"{domain}/*", "output": "json", "limit": page_limit}
    query_url = COMMONCRAWL_QUERY_TEMPLATE.format(index_id=index_id)

    resp = None
    for attempt in range(1, max_retries + 1):
        try:
            resp = session.get(query_url, params=params, timeout=60)
        except requests.RequestException as exc:
            print(f"  [warn] CDX query failed for {index_id} / {domain} (attempt {attempt}): {exc}")
            resp = None
        else:
            if resp.status_code == 404:
                return set()
            if resp.status_code < 500:
                break
            print(f"  [warn] CDX query got {resp.status_code} for {index_id} / {domain} (attempt {attempt})")
        if attempt < max_retries:
            time.sleep(2 ** attempt)

    if resp is None or resp.status_code >= 400:
        print(f"  [warn] giving up on {index_id} / {domain} after {max_retries} attempts")
        return set()

    slugs, malformed = parse_cdx_lines(resp.text, domain)
    if malformed:
        print(f"  [warn] skipped {malformed} malformed CDX line(s) for {index_id} / {domain}")
    return slugs


def fetch_wayback_slugs_for_prefix(session: requests.Session, domain: str, prefix: str, page_limit: int,
                                    max_retries: int = 3) -> set[str]:
    from_date = (datetime.now(timezone.utc) - timedelta(days=WAYBACK_LOOKBACK_DAYS)).strftime("%Y%m%d")
    params = {
        "url": f"{domain}/{prefix}*",
        "output": "json",
        "filter": "statuscode:200",
        "collapse": "urlkey",
        "from": from_date,
        "limit": page_limit,
    }

    resp = None
    for attempt in range(1, max_retries + 1):
        try:
            resp = session.get(WAYBACK_QUERY_URL, params=params, timeout=60)
        except requests.RequestException as exc:
            print(f"    [warn] Wayback CDX query failed for {domain}/{prefix}* (attempt {attempt}): {exc}")
            resp = None
        else:
            if resp.status_code < 500:
                break
            print(f"    [warn] Wayback CDX query got {resp.status_code} for {domain}/{prefix}* (attempt {attempt})")
        if attempt < max_retries:
            time.sleep(2 ** attempt)

    if resp is None or resp.status_code >= 400:
        print(f"    [warn] giving up on Wayback / {domain}/{prefix}* after {max_retries} attempts")
        return set()

    try:
        rows = resp.json()
    except ValueError:
        print(f"    [warn] Wayback CDX returned non-JSON for {domain}/{prefix}*")
        return set()

    if not rows:
        return set()

    header, *records = rows  # first row is the column header, e.g. ["urlkey","timestamp","original",...]
    url_idx = header.index("original") if "original" in header else 2

    slugs: set[str] = set()
    for record in records:
        slug = extract_slug(record[url_idx], domain)
        if slug:
            slugs.add(slug)
    return slugs


def discover_candidate_slugs(spec: ATSSpec, num_indexes: int, max_slugs: int, page_limit: int) -> list[str]:
    session = requests.Session()
    all_slugs: set[str] = set()

    if spec.cdx_source == CDX_SOURCE_COMMONCRAWL:
        index_ids = get_latest_commoncrawl_index_ids(num_indexes)
        print(f"  Using Common Crawl indexes: {', '.join(index_ids)}")
        for index_id in index_ids:
            for domain in spec.cdx_domains:
                print(f"  querying {domain} in {index_id} ...")
                found = fetch_commoncrawl_slugs(session, index_id, domain, page_limit)
                new_count = len(found - all_slugs)
                all_slugs |= found
                print(f"    -> {len(found)} slugs ({new_count} new, {len(all_slugs)} total so far)")
                if len(all_slugs) >= max_slugs:
                    break
            if len(all_slugs) >= max_slugs:
                break

    elif spec.cdx_source == CDX_SOURCE_WAYBACK:
        print(f"  Using Wayback Machine CDX (last {WAYBACK_LOOKBACK_DAYS} days), queried per prefix - "
              f"Common Crawl's bot is blocked by {spec.key}'s robots.txt")
        prefix_page_limit = max(page_limit // len(WAYBACK_SLUG_PREFIXES), 50)
        for domain in spec.cdx_domains:
            for prefix in WAYBACK_SLUG_PREFIXES:
                found = fetch_wayback_slugs_for_prefix(session, domain, prefix, prefix_page_limit)
                new_count = len(found - all_slugs)
                all_slugs |= found
                print(f"  {domain}/{prefix}* -> {len(found)} slugs ({new_count} new, {len(all_slugs)} total so far)")
                if len(all_slugs) >= max_slugs:
                    break
            if len(all_slugs) >= max_slugs:
                break
    else:
        raise ValueError(f"unknown cdx_source: {spec.cdx_source}")

    return sorted(all_slugs)[:max_slugs]


def validate_slug(session: requests.Session, spec: ATSSpec, slug: str) -> dict:
    """Deterministic validation against the ATS's own public API. No LLM involved."""
    url = spec.validate_url_template.format(slug=slug)
    result = {
        "ats": spec.key,
        "slug": slug,
        "checked_url": url,
        "status": "invalid",
        "company_name": "",
        "job_count": 0,
        "usa_job_count": 0,
        "jobs": [],
    }
    try:
        resp = session.get(url, timeout=15)
    except requests.RequestException as exc:
        result["status"] = f"error:{exc.__class__.__name__}"
        return result

    if resp.status_code != 200:
        result["status"] = "invalid"
        return result

    try:
        data = resp.json()
    except ValueError:
        result["status"] = "invalid"
        return result

    parsed = spec.parse_validation(data, slug)
    jobs = parsed["jobs"]
    result["job_count"] = len(jobs)
    if not jobs:
        result["status"] = "empty"
        return result

    result["status"] = "valid"
    result["company_name"] = parsed["company_name"] or slug
    result["jobs"] = jobs
    result["usa_job_count"] = sum(1 for j in jobs if j["classification"] == "usa")
    return result


def validate_candidates(spec: ATSSpec, slugs: list[str], delay_seconds: float) -> list[dict]:
    session = requests.Session()
    results = []
    for i, slug in enumerate(slugs, start=1):
        result = validate_slug(session, spec, slug)
        results.append(result)
        print(f"  [{spec.key} {i}/{len(slugs)}] {slug}: {result['status']} "
              f"({result['company_name'] or '-'}, {result['job_count']} jobs, "
              f"{result['usa_job_count']} usa)")
        if i < len(slugs):
            time.sleep(delay_seconds)
    return results


def write_csv(path: Path, rows: list[dict], fieldnames: list[str]) -> None:
    with path.open("w", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=fieldnames, extrasaction="ignore")
        writer.writeheader()
        writer.writerows(rows)


def run_ats(spec: ATSSpec, args: argparse.Namespace, timestamp: str) -> dict:
    print(f"\n=== {spec.key}: extracting candidate slugs ===")
    slugs = discover_candidate_slugs(spec, args.num_indexes, args.max_slugs, args.page_limit)
    print(f"Found {len(slugs)} distinct candidate slugs to validate.\n")

    print(f"=== {spec.key}: validating candidates against the public job board API ===")
    results = validate_candidates(spec, slugs, args.delay)

    all_path = OUTPUT_DIR / f"{spec.key}_candidates_{timestamp}.csv"
    write_csv(all_path, results, ["ats", "slug", "status", "company_name", "job_count", "checked_url"])

    valid_rows = [r for r in results if r["status"] == "valid"]
    valid_path = OUTPUT_DIR / f"{spec.key}_valid_companies_{timestamp}.csv"
    write_csv(valid_path, valid_rows,
              ["ats", "slug", "company_name", "job_count", "usa_job_count", "checked_url"])

    usa_job_rows = [
        {
            "ats": spec.key,
            "company_name": r["company_name"],
            "slug": r["slug"],
            "title": job["title"],
            "location": job["location"],
            "absolute_url": job["absolute_url"],
        }
        for r in valid_rows
        for job in r["jobs"]
        if job["classification"] == "usa"
    ]
    usa_jobs_path = OUTPUT_DIR / f"{spec.key}_usa_jobs_{timestamp}.csv"
    write_csv(usa_jobs_path, usa_job_rows,
              ["ats", "company_name", "slug", "title", "location", "absolute_url"])

    status_counts: dict[str, int] = {}
    for r in results:
        status_counts[r["status"]] = status_counts.get(r["status"], 0) + 1
    total_jobs = sum(r["job_count"] for r in valid_rows)

    print(f"\n--- {spec.key} summary ---")
    for status, count in sorted(status_counts.items()):
        print(f"  {status}: {count}")
    print(f"  usa jobs / total jobs (valid companies): {len(usa_job_rows)} / {total_jobs}")
    print(f"  candidates:      {all_path}")
    print(f"  valid companies: {valid_path}")
    print(f"  usa jobs:        {usa_jobs_path}")

    return {
        "valid_companies": len(valid_rows),
        "total_jobs": total_jobs,
        "usa_jobs": len(usa_job_rows),
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--ats", choices=[*ATS_SPECS.keys(), "all"], default="all",
                         help="which ATS to run discovery for (default: all)")
    parser.add_argument("--max-slugs", type=int, default=300,
                         help="max distinct candidate slugs to validate, per ATS (default: 300)")
    parser.add_argument("--num-indexes", type=int, default=3,
                         help="how many recent Common Crawl indexes to query (default: 3; ignored for Lever)")
    parser.add_argument("--page-limit", type=int, default=2000,
                         help="max CDX records fetched per query (default: 2000)")
    parser.add_argument("--delay", type=float, default=0.4,
                         help="seconds to sleep between validation API calls (default: 0.4)")
    args = parser.parse_args()

    OUTPUT_DIR.mkdir(exist_ok=True)
    timestamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")

    ats_keys = list(ATS_SPECS.keys()) if args.ats == "all" else [args.ats]
    summary = {}
    for key in ats_keys:
        summary[key] = run_ats(ATS_SPECS[key], args, timestamp)

    print("\n=== Overall summary ===")
    for key, s in summary.items():
        print(f"  {key}: {s['valid_companies']} valid companies, "
              f"{s['usa_jobs']}/{s['total_jobs']} usa jobs")


if __name__ == "__main__":
    main()
