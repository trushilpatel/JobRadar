"""
Per-ATS configuration for the discovery CLI. Adding a new ATS is meant to be
one new ATSSpec entry here - discover.py itself has no per-ATS branching.

Extraction source note:
  - Greenhouse and Ashby allow Common Crawl's bot (CCBot) per robots.txt, so
    their boards are well covered by the Common Crawl CDX index.
  - Lever's robots.txt explicitly disallows CCBot (`User-agent: CCBot /
    Disallow: /`), so Common Crawl has almost no coverage of jobs.lever.co.
    The Wayback Machine's crawler is not blocked, so Lever uses the Internet
    Archive's CDX API instead - same query shape, still free, still no LLM.
"""

from dataclasses import dataclass
from typing import Callable

from location_classifier import classify as classify_location

CDX_SOURCE_COMMONCRAWL = "commoncrawl"
CDX_SOURCE_WAYBACK = "wayback"


@dataclass(frozen=True)
class ATSSpec:
    key: str
    cdx_source: str
    cdx_domains: list[str]
    validate_url_template: str
    parse_validation: Callable[[object, str], dict]


def _normalize_job(title: str, location: str, url: str) -> dict:
    return {
        "title": title or "",
        "location": location or "",
        "classification": classify_location(location or ""),
        "absolute_url": url or "",
    }


def parse_greenhouse(data: dict, slug: str) -> dict:
    jobs = data.get("jobs") or []
    company_name = jobs[0].get("company_name", "") or slug if jobs else ""
    normalized = [
        _normalize_job(
            j.get("title", ""),
            (j.get("location") or {}).get("name", ""),
            j.get("absolute_url", ""),
        )
        for j in jobs
    ]
    return {"company_name": company_name, "jobs": normalized}


def parse_ashby(data: dict, slug: str) -> dict:
    jobs = data.get("jobs") or []
    # Ashby's public job-board endpoint carries no company/organization name
    # field anywhere in the payload - only the slug is available.
    company_name = slug.replace("-", " ").replace("_", " ").title()
    normalized = [
        _normalize_job(j.get("title", ""), j.get("location", ""), j.get("jobUrl", ""))
        for j in jobs
    ]
    return {"company_name": company_name, "jobs": normalized}


def parse_lever(data: object, slug: str) -> dict:
    postings = data if isinstance(data, list) else []
    # Like Ashby, Lever's public postings endpoint has no company display
    # name field on the response - only the slug is available.
    company_name = slug.replace("-", " ").replace("_", " ").title()
    normalized = [
        _normalize_job(
            p.get("text", ""),
            (p.get("categories") or {}).get("location", ""),
            p.get("hostedUrl", ""),
        )
        for p in postings
    ]
    return {"company_name": company_name, "jobs": normalized}


ATS_SPECS: dict[str, ATSSpec] = {
    "greenhouse": ATSSpec(
        key="greenhouse",
        cdx_source=CDX_SOURCE_COMMONCRAWL,
        cdx_domains=["boards.greenhouse.io", "job-boards.greenhouse.io"],
        validate_url_template="https://boards-api.greenhouse.io/v1/boards/{slug}/jobs",
        parse_validation=parse_greenhouse,
    ),
    "ashby": ATSSpec(
        key="ashby",
        cdx_source=CDX_SOURCE_COMMONCRAWL,
        cdx_domains=["jobs.ashbyhq.com"],
        validate_url_template="https://api.ashbyhq.com/posting-api/job-board/{slug}",
        parse_validation=parse_ashby,
    ),
    "lever": ATSSpec(
        key="lever",
        cdx_source=CDX_SOURCE_WAYBACK,
        cdx_domains=["jobs.lever.co"],
        validate_url_template="https://api.lever.co/v0/postings/{slug}?mode=json",
        parse_validation=parse_lever,
    ),
}
