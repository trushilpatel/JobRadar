#!/usr/bin/env python3
"""
validate_ats.py — Fast concurrent ATS validator for Greenhouse, Ashby, Lever.

Hits each company's actual API endpoint, compares real job count vs stored,
flags broken slugs and wrong ATS classifications.

Usage:
    python3 validate_ats.py [--ats greenhouse|ashby|lever|all] [--workers 40] [--out results.csv]
"""

import json
import csv
import time
import argparse
import urllib.request
import urllib.error
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass, field, asdict
from typing import Optional
from pathlib import Path

# ── Config ────────────────────────────────────────────────────────────────────

DATA_FILE   = Path(__file__).parent.parent / "outputs" / "combined_filtered.json"
DEFAULT_OUT = Path(__file__).parent / "ats_validation.csv"

HEADERS = {
    "User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
    "Accept": "application/json",
}

TIMEOUT = 12  # seconds per request

# ── Result dataclass ──────────────────────────────────────────────────────────

@dataclass
class Result:
    company:     str
    domain:      str
    ats:         str
    slug:        str
    api_url:     str
    stored_jobs: int
    actual_jobs: int    = -1
    status:      str    = ""   # ok | http_404 | http_NNN | error | no_slug | empty
    note:        str    = ""
    investors:   str    = ""

    @property
    def delta(self) -> int:
        return self.actual_jobs - self.stored_jobs if self.actual_jobs >= 0 else 0

    @property
    def ok(self) -> bool:
        return self.status == "ok"


# ── Slug extraction ───────────────────────────────────────────────────────────

def slug_from_url(ats_url: str) -> Optional[str]:
    """Extract the last path component of the ATS URL as the slug."""
    if not ats_url:
        return None
    return ats_url.rstrip("/").rsplit("/", 1)[-1] or None


# ── Per-ATS checkers ──────────────────────────────────────────────────────────

def check_greenhouse(c: dict) -> Result:
    slug = slug_from_url(c["U"])
    api_url = f"https://boards-api.greenhouse.io/v1/boards/{slug}/jobs?content=false" if slug else ""

    r = Result(
        company=c["C"], domain=c["D"], ats="Greenhouse",
        slug=slug or "", api_url=api_url,
        stored_jobs=c["J"], investors=c["V"]
    )

    if not slug:
        r.status = "no_slug"
        return r

    try:
        req = urllib.request.Request(api_url, headers=HEADERS)
        with urllib.request.urlopen(req, timeout=TIMEOUT) as resp:
            data = json.loads(resp.read())
            jobs = data.get("jobs", [])
            r.actual_jobs = len(jobs)
            r.status = "ok"
            if r.actual_jobs == 0:
                r.status = "empty"
                r.note = "API returned 0 jobs"
    except urllib.error.HTTPError as e:
        r.status = f"http_{e.code}"
        if e.code == 404:
            r.note = "Slug not found — likely dashes stripped wrong or wrong ATS"
    except urllib.error.URLError as e:
        r.status = "error"
        r.note = str(e.reason)[:80]
    except Exception as e:
        r.status = "error"
        r.note = str(e)[:80]

    return r


def check_lever(c: dict) -> Result:
    slug = slug_from_url(c["U"])
    api_url = f"https://api.lever.co/v0/postings/{slug}?mode=json&limit=1" if slug else ""

    r = Result(
        company=c["C"], domain=c["D"], ats="Lever",
        slug=slug or "", api_url=api_url,
        stored_jobs=c["J"], investors=c["V"]
    )

    if not slug:
        r.status = "no_slug"
        return r

    # First check: lightweight count via limit=1 then read total from Link header
    # Actually Lever returns an array — just count it (no pagination header for count)
    # Use limit=0 trick: Lever ignores limit=0 and returns all, so we'll use limit=500
    count_url = f"https://api.lever.co/v0/postings/{slug}?mode=json"

    try:
        req = urllib.request.Request(count_url, headers=HEADERS)
        with urllib.request.urlopen(req, timeout=TIMEOUT) as resp:
            data = json.loads(resp.read())
            if not isinstance(data, list):
                r.status = "error"
                r.note = f"Unexpected response type: {type(data).__name__}"
                return r
            r.actual_jobs = len(data)
            r.status = "ok" if r.actual_jobs > 0 else "empty"
            if r.actual_jobs == 0:
                r.note = "API returned 0 postings"
    except urllib.error.HTTPError as e:
        r.status = f"http_{e.code}"
        if e.code == 404:
            r.note = "Company slug not found on Lever"
    except urllib.error.URLError as e:
        r.status = "error"
        r.note = str(e.reason)[:80]
    except Exception as e:
        r.status = "error"
        r.note = str(e)[:80]

    return r


def check_ashby(c: dict) -> Result:
    slug = slug_from_url(c["U"])
    api_url = "https://jobs.ashbyhq.com/api/non-user-graphql?op=ApiJobBoardWithTeams"

    r = Result(
        company=c["C"], domain=c["D"], ats="Ashby",
        slug=slug or "", api_url=api_url,
        stored_jobs=c["J"], investors=c["V"]
    )

    if not slug:
        r.status = "no_slug"
        return r

    payload = json.dumps({
        "operationName": "ApiJobBoardWithTeams",
        "variables": {"organizationHostedJobsPageName": slug},
        "query": """query ApiJobBoardWithTeams($organizationHostedJobsPageName: String!) {
            jobBoardWithTeams(organizationHostedJobsPageName: $organizationHostedJobsPageName) {
                jobPostings { id }
            }
        }"""
    }).encode()

    try:
        req = urllib.request.Request(
            api_url, data=payload,
            headers={**HEADERS, "Content-Type": "application/json"},
            method="POST"
        )
        with urllib.request.urlopen(req, timeout=TIMEOUT) as resp:
            data = json.loads(resp.read())
            board = data.get("data", {}).get("jobBoardWithTeams")
            if board is None:
                r.status = "not_found"
                r.note = "jobBoardWithTeams returned null — slug wrong or company not on Ashby"
                return r
            postings = board.get("jobPostings", [])
            r.actual_jobs = len(postings)
            r.status = "ok" if r.actual_jobs > 0 else "empty"
            if r.actual_jobs == 0:
                r.note = "API returned 0 job postings"
    except urllib.error.HTTPError as e:
        r.status = f"http_{e.code}"
        r.note = str(e)[:80]
    except urllib.error.URLError as e:
        r.status = "error"
        r.note = str(e.reason)[:80]
    except Exception as e:
        r.status = "error"
        r.note = str(e)[:80]

    return r


CHECKERS = {
    "Greenhouse": check_greenhouse,
    "Lever":      check_lever,
    "Ashby":      check_ashby,
}


# ── Main ──────────────────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--ats",     default="all", help="greenhouse|ashby|lever|all")
    parser.add_argument("--workers", type=int, default=40)
    parser.add_argument("--out",     default=str(DEFAULT_OUT))
    args = parser.parse_args()

    # Load data
    with open(DATA_FILE) as f:
        all_companies = json.load(f)

    target = {k for k in CHECKERS} if args.ats == "all" else {args.ats.capitalize()}
    companies = [c for c in all_companies if c["A"] in target]

    print(f"\n{'='*60}")
    print(f"  ATS Validator  —  {len(companies)} companies  ({', '.join(sorted(target))})")
    print(f"  Workers: {args.workers}  |  Timeout: {TIMEOUT}s per request")
    print(f"{'='*60}\n")

    results: list[Result] = []
    start = time.time()
    done = 0

    with ThreadPoolExecutor(max_workers=args.workers) as pool:
        futures = {
            pool.submit(CHECKERS[c["A"]], c): c
            for c in companies if c["A"] in CHECKERS
        }
        for fut in as_completed(futures):
            done += 1
            r = fut.result()
            results.append(r)
            icon = "✓" if r.ok else "✗" if "404" in r.status or r.status in ("not_found","no_slug") else "~"
            print(f"  [{done:>4}/{len(futures)}] {icon} {r.ats:<12} {r.company[:35]:<35} "
                  f"stored={r.stored_jobs:>4}  actual={r.actual_jobs if r.actual_jobs >= 0 else '?':>4}  {r.status}")

    elapsed = time.time() - start
    print(f"\nDone in {elapsed:.1f}s\n")

    # ── Summary ───────────────────────────────────────────────────────────────
    from collections import Counter

    by_ats = {}
    for ats in sorted(target):
        grp = [r for r in results if r.ats == ats]
        statuses = Counter(r.status for r in grp)
        ok      = statuses.get("ok", 0)
        empty   = statuses.get("empty", 0)
        missing = statuses.get("http_404", 0) + statuses.get("not_found", 0)
        errors  = sum(v for k, v in statuses.items() if k not in ("ok","empty","http_404","not_found","no_slug"))
        no_slug = statuses.get("no_slug", 0)
        by_ats[ats] = dict(total=len(grp), ok=ok, empty=empty, missing=missing, errors=errors, no_slug=no_slug)

        print(f"  {ats}")
        print(f"    Total:    {len(grp)}")
        print(f"    ✓ OK:     {ok}  ({ok*100//len(grp) if grp else 0}%)")
        print(f"    ~ Empty:  {empty}")
        print(f"    ✗ 404:    {missing}  ← wrong slug / not on this ATS")
        print(f"    ✗ Error:  {errors}")
        print(f"    ? No slug:{no_slug}")
        print()

    # ── Write CSV ─────────────────────────────────────────────────────────────
    out_path = Path(args.out)
    fields = ["company","domain","ats","slug","status","actual_jobs","stored_jobs","delta","note","investors","api_url"]

    with open(out_path, "w", newline="") as f:
        w = csv.DictWriter(f, fieldnames=fields)
        w.writeheader()
        for r in sorted(results, key=lambda x: (x.ats, x.status, x.company)):
            row = asdict(r)
            row["delta"] = r.delta
            w.writerow({k: row[k] for k in fields})

    print(f"  Results → {out_path}")
    print(f"  Broken (404/not_found): {[r.company for r in results if '404' in r.status or r.status == 'not_found'][:20]}")


if __name__ == "__main__":
    main()
