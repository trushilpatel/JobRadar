package prompts

import (
	"fmt"
	"strings"
)

// BuildScoreBatch returns the prompt for batch job relevance scoring.
func BuildScoreBatch(roles []string, yearsExp int, jobList string) string {
	return fmt.Sprintf(`Return ONLY a valid JSON array of strings. DO NOT use markdown. NO preamble.

You are a job relevance filter for a software engineer with the following profile:
- Target roles: %s
- Years of experience: %d+ years

Evaluate each job title and location to return the IDs of jobs that are a plausible match.

SENIORITY GUIDELINES
INCLUDE: Junior, Mid-level, Senior, Software Engineer I/II/III, SWE, Backend, Frontend, Fullstack, Platform, Infrastructure, DevOps, SRE, Data Engineer, ML Engineer, Research Engineer, Research Scientist, Data Scientist, Applied Scientist, Solutions Architect, Cloud Architect, Data Platform Architect, and similar computer science IC roles.
EXCLUDE: Staff [X], Principal [X], Distinguished, Fellow, Director, VP, C-suite, Head of Engineering, or any role that is explicitly a people manager with no IC track. This includes titles like "Staff Software Engineer", "Principal Software Engineer" — the Staff/Principal prefix overrides the engineering keyword.
EXCLUDE: Internships, Co-ops, Apprenticeships, part-time.
EXCLUDE: Non-CS roles — Sales, Account Executive, Account Manager, Account Director, Pre-Sales, Marketing, Recruiting, Finance, Legal, Customer Success, Solutions Consultant, Delivery Consultant, Implementation Consultant, Program Manager, Technical Program Manager, SDR, Operations Manager, Field Operations, Communications Lead, Strategy, Brand.
EXCLUDE: Non-CS engineering — Mechanical Engineer, Electrical Engineer, Turbomachinery, Hardware Assembly, Civil Engineer, or any engineering role not related to computer science or software systems.
EXCLUDE: Non-CS analyst roles — Payroll Analyst, Workday Analyst, Finance Analyst, Compliance Analyst, Audio Analyst, or any analyst role not related to software, data, or security.
EXCLUDE: Systems Administrator, IT Support.

LOCATION GUIDELINES
INCLUDE: Positions with a US city, US state, or "United States" in the location.
INCLUDE: Positions listed as "Remote" or "Remote - USA" or similar US-remote variants.
INCLUDE: Positions with no location or "Unknown" location — assume US.
EXCLUDE: Positions with a non-US city or country in the location (e.g. London, Toronto, Berlin, Sydney, India, Canada).
EXCLUDE: Positions that are explicitly "Remote - [non-US country]" or list only non-US locations.
EXCLUDE: Positions with multi-region location strings like "Home based - EMEA", "Home based - Worldwide", "Americas; EMEA", or similar patterns that do not explicitly include the US.

When in doubt, EXCLUDE the job. Precision matters — irrelevant jobs that pass here waste enrichment budget and pollute the feed.

Return a JSON array of matching job IDs. Return an empty array if none match.

Job list:
%s`, strings.Join(roles, ", "), yearsExp, jobList)
}
