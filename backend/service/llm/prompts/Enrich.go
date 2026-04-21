package prompts

import (
	"fmt"
	"strings"
)

// BuildEnrich returns the prompt for structured job post data extraction.
func BuildEnrich(jobID string, roles []string, rawText string) string {
	targetRoles := joinRoles(roles)
	return fmt.Sprintf(`Return ONLY raw JSON. No markdown formatting.

Extract structured information from the following job posting. Strictly evaluate the text provided without using external knowledge or making assumptions about company policies not explicitly stated in the content.

Job ID: %s
Target roles: %s

Job posting text:
%s

Field instructions:
- experience_years: Extract the minimum years of experience as an integer. Recognize all common phrasings: "3-5 years" → 3, "3+ years" → 3, "at least 3 years" → 3, "minimum 3 years" → 3, "3 to 5 years" → 3, "5 or more years" → 5. Treat "preferred", "ideally", "typically", and "we're looking for" the same as "required" — extract the number regardless. If no explicit number appears, infer from seniority: Entry/Junior/Associate/New Grad → 0, Mid/Mid-Level → 3, Senior/Sr → 5, Staff/Principal → 7. Return null only if no number and no seniority indicator appears anywhere in the posting.
- tech_stack: array of specific programming languages, frameworks, databases, cloud platforms, and developer tools explicitly named in the posting. Include: languages (Go, Python, Java, TypeScript), frameworks (React, Django, Spring Boot), databases (PostgreSQL, Redis, MongoDB), cloud services (AWS, GCP, Azure), and dev tools (Docker, Kubernetes, Terraform, Git). Exclude: methodologies (Agile, Scrum, Kanban), soft skills (Communication, Leadership), architectural patterns (REST, MVC, Microservices, CI/CD), and generic office tools (Excel, Word, Microsoft Office). Use canonical names (e.g. "PostgreSQL" not "Postgres", "React" not "ReactJS", "Node.js" not "Node").
- salary_min, salary_max: integers or null if not stated
- salary_currency: Extract salary or salary range. if not found null.
- summary: 2-3 sentence summary of the role only, do not include any information about company
- remote: true if remote or hybrid, false if fully on-site
- country: country where the candidate must be based. Use "USA" for any US city, US state, or "United States". Use the country name (e.g. "Canada", "UK") if the posting explicitly restricts to a non-US location. Empty string only if the posting has no location restriction at all (worldwide remote, no country mentioned).
- state: US state abbreviation if restricted (e.g. "CA"), null otherwise
- sponsorship: false ONLY if the posting contains explicit language such as "no sponsorship", "US person", "U.S. export controlled", "will not sponsor", "sponsorship not available", "must be authorized to work", "does not offer visa sponsorship", or similar denial phrases. Default true if not mentioned.
- requires_citizen: true ONLY if the posting explicitly states "US Citizen", "US Citizenship required", "must be a US citizen", "Green Card required", or "must have permanent residency". Default false if not mentioned.
- security_clearance: true ONLY if the posting explicitly states that an active security clearance is required to perform the job (e.g. "Secret clearance required", "TS/SCI required", "active clearance required", "must hold clearance", "clearance is mandatory"). Default false if clearance is not mentioned, is described as preferred or a bonus, or is something the employer will sponsor the candidate to obtain ("ability to obtain", "eligible to obtain clearance", "will obtain clearance", "willingness to obtain").
- role_brief: one sentence technical mission of the role
- responsibilities: array of concise strings, one key technical responsibility per element, no noise
- role_category: one of backend, frontend, fullstack, devops, ml, data, other
- role_match: true if this job is a genuine match for the target roles (%s). Evaluate the full job title, responsibilities, and required skills. Return false if the role is clearly non-engineering (e.g. Sales, Account Executive, Program Manager, Marketing), non-CS engineering (e.g. Mechanical, Electrical), or requires a specialisation unrelated to the target roles. When genuinely ambiguous, return true.`,
		jobID, targetRoles, rawText, targetRoles)
}

func joinRoles(roles []string) string {
	return strings.Join(roles, ", ")
}
