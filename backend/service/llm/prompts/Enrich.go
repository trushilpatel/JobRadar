package prompts

import (
	"fmt"
	"strings"

	"github.com/trushilpatel/jobradar/service/llm/dto"
)

// BuildEnrichBatch returns the prompt for structured extraction across one or
// more job postings in a single LLM call. The model returns a JSON array of
// result objects, each carrying the matching job_id so callers can attribute
// results back to their source by ID rather than by array position.
//
// A batch of size 1 is the natural single-job case — same prompt shape, same
// schema, just one element in the input and output arrays.
func BuildEnrichBatch(roles []string, inputs []dto.JobPostEnrichInput) string {
	targetRoles := joinRoles(roles)

	var jobsBlock strings.Builder
	for i, in := range inputs {
		if i > 0 {
			jobsBlock.WriteString("\n---\n")
		}
		fmt.Fprintf(&jobsBlock, "Job ID: %s\nJob posting text:\n%s", in.ID, in.RawText)
	}

	return fmt.Sprintf(`Return ONLY raw JSON. No markdown formatting.

Extract structured information from each of the following job postings. Strictly evaluate the text provided without using external knowledge or making assumptions about company policies not explicitly stated in the content.

Target roles: %s

Return a JSON ARRAY containing one result object per input job. Each result object MUST include the matching "job_id" field so the caller can correlate results to inputs. Preserve input order in the output. Do not add or omit jobs.

Jobs to process:
%s

Field instructions (apply per job):
- job_id: copy the matching Job ID from the input verbatim — this is how the caller maps results to inputs
- experience_years: Extract the minimum years of experience as an integer. Recognize all common phrasings: "3-5 years" → 3, "3+ years" → 3, "at least 3 years" → 3, "minimum 3 years" → 3, "3 to 5 years" → 3, "5 or more years" → 5. Treat "preferred", "ideally", "typically", and "we're looking for" the same as "required" — extract the number regardless. If no explicit number appears, infer from seniority: Entry/Junior/Associate/New Grad → 0, Mid/Mid-Level → 3, Senior/Sr → 5, Staff/Principal → 7. Return null only if no number and no seniority indicator appears anywhere in the posting.
- tech_stack: array of ATS-scannable keywords explicitly named in the posting — the exact skills, tools, and qualifications a resume-screening system would search for — across these categories — (1) programming languages, frameworks, databases, cloud platforms, and developer tools (Go, Python, React, PostgreSQL, AWS, Docker, Terraform, Git); (2) methodologies and practices (Agile, Scrum, Kanban, CI/CD, TDD, Microservices, MVC, REST); (3) non-developer tools and platforms explicitly named (Jira, Confluence, Figma, Salesforce, Tableau); (4) soft skills explicitly named (Leadership, Communication, Stakeholder management, Mentorship); (5) certifications and domain terms explicitly named (AWS Certified, PMP, HIPAA, fintech, healthcare); (6) qualifications, ONLY when a specific field of study is named (e.g. "Bachelor's in Computer Science", "Master's in Data Science") — never extract a fieldless degree mention like "Bachelor's degree" or "college degree" on its own, since that appears in nearly every posting and adds no signal. Prefer the most specific named entity actually used in the posting: when a specific product or service is named (e.g. "AWS Lambda", "Node.js", "Kubernetes"), extract that specific name instead of — not in addition to — its generic parent (do not also add "AWS" or "cloud" alongside "AWS Lambda" for the same mention). Each keyword must be a short, specific named entity of 1-4 words, never a full clause or sentence copied from the posting. Exclude only generic, non-specific terms: generic office tools (Excel, Word, Microsoft Office), bare acronyms with no specific referent (API, SDK, JSON, HTML on their own), generic OS names (Windows, macOS), and fieldless degree mentions. Use canonical names (e.g. "PostgreSQL" not "Postgres", "React" not "ReactJS", "CI/CD" not "continuous integration") — canonical means the specific name actually used in the posting, not a generic substitute for it.
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
		targetRoles, jobsBlock.String(), targetRoles)
}

func joinRoles(roles []string) string {
	return strings.Join(roles, ", ")
}
