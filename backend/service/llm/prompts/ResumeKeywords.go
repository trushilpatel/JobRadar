package prompts

import "fmt"

// BuildResumeKeywords returns the prompt for extracting technology/skill
// keywords from a resume's plain text body. Truncation to a sane token
// budget (service/llm/common.ResumeKeywordMaxTextChars) happens in the
// caller (geminiScorer.ExtractResumeKeywords) before this is invoked — this
// function only shapes the prompt around whatever text it is given.
func BuildResumeKeywords(text string) string {
	return fmt.Sprintf(`Return ONLY raw JSON. No markdown formatting.

Extract technology and skill keywords from the following resume text. Strictly evaluate the text provided without using external knowledge or making assumptions.

Return a JSON array of strings. Include ONLY: programming languages (Go, Python, Java, TypeScript), frameworks and libraries (React, Django, Spring Boot), databases (PostgreSQL, Redis, MongoDB), cloud platforms and services (AWS, GCP, Azure, Kubernetes), developer tools and infrastructure (Docker, Terraform, Git, Kafka). Use canonical names (e.g. "PostgreSQL" not "Postgres", "React" not "ReactJS").

Exclude: soft skills (communication, leadership, teamwork), generic office tools (Excel, Word, PowerPoint), methodologies (Agile, Scrum), job titles, company names, and any word that is not a specific named technology, framework, tool, or platform.

Return an empty array if no technology keywords are found. Do not invent keywords that are not present in the text.

Resume text:
%s`, text)
}
