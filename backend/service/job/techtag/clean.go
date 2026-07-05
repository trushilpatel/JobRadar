// Package techtag normalizes and filters raw tech_stack strings extracted by the LLM.
// It maps variant spellings to canonical names and drops generic noise — see
// `canonical` and `blocklist` for what counts as noise vs. an allowed keyword
// (which now spans tech stack, methodologies, tools, soft skills, and
// certifications).
package techtag

import "strings"

// canonical maps lowercase variant spellings to their canonical tech name.
var canonical = map[string]string{
	// JavaScript ecosystem
	"javascript": "JavaScript",
	"js":         "JavaScript",
	"typescript": "TypeScript",
	"ts":         "TypeScript",
	"react":      "React",
	"reactjs":    "React",
	"react.js":   "React",
	"vue":        "Vue",
	"vuejs":      "Vue",
	"vue.js":     "Vue",
	"angular":    "Angular",
	"angularjs":  "Angular",
	"angular.js": "Angular",
	"nodejs":     "Node.js",
	"node.js":    "Node.js",
	"node":       "Node.js",
	"nextjs":     "Next.js",
	"next.js":    "Next.js",
	"nuxtjs":     "Nuxt.js",
	"nuxt.js":    "Nuxt.js",
	"express":    "Express",
	"expressjs":  "Express",
	"express.js": "Express",
	// Python
	"python":  "Python",
	"py":      "Python",
	"fastapi": "FastAPI",
	"django":  "Django",
	"flask":   "Flask",
	// Go
	"go":     "Go",
	"golang": "Go",
	// Databases
	"postgresql":     "PostgreSQL",
	"postgres":       "PostgreSQL",
	"mysql":          "MySQL",
	"sqlite":         "SQLite",
	"mongodb":        "MongoDB",
	"mongo":          "MongoDB",
	"redis":          "Redis",
	"elasticsearch":  "Elasticsearch",
	"elastic search": "Elasticsearch",
	"cassandra":      "Cassandra",
	"dynamodb":       "DynamoDB",
	"dynamo db":      "DynamoDB",
	// Cloud
	"amazon web services":   "AWS",
	"aws":                   "AWS",
	"google cloud platform": "GCP",
	"google cloud":          "GCP",
	"gcp":                   "GCP",
	"microsoft azure":       "Azure",
	"azure":                 "Azure",
	// Container / infra
	"kubernetes": "Kubernetes",
	"k8s":        "Kubernetes",
	"docker":     "Docker",
	"terraform":  "Terraform",
	"helm":       "Helm",
	// Other common
	"graphql":       "GraphQL",
	"grpc":          "gRPC",
	"kafka":         "Kafka",
	"rabbitmq":      "RabbitMQ",
	"git":           "Git",
	"github":        "GitHub",
	"gitlab":        "GitLab",
	"java":          "Java",
	"kotlin":        "Kotlin",
	"scala":         "Scala",
	"rust":          "Rust",
	"c++":           "C++",
	"c#":            "C#",
	"dotnet":        ".NET",
	".net":          ".NET",
	"ruby":          "Ruby",
	"rails":         "Rails",
	"ruby on rails": "Rails",
	"swift":         "Swift",
	"spring":        "Spring",
	"spring boot":   "Spring Boot",
	"springboot":    "Spring Boot",
	// Methodologies / practices
	"agile":                  "Agile",
	"scrum":                  "Scrum",
	"kanban":                 "Kanban",
	"waterfall":              "Waterfall",
	"lean":                   "Lean",
	"rest":                   "REST",
	"restful":                "REST",
	"soap":                   "SOAP",
	"mvc":                    "MVC",
	"mvvm":                   "MVVM",
	"microservices":          "Microservices",
	"serverless":             "Serverless",
	"solid":                  "SOLID",
	"oop":                    "OOP",
	"object-oriented":        "OOP",
	"functional programming": "Functional Programming",
	"ci/cd":                  "CI/CD",
	"ci cd":                  "CI/CD",
	"devops":                 "DevOps",
	// Non-dev tools / platforms
	"jira":       "Jira",
	"confluence": "Confluence",
	"figma":      "Figma",
	"salesforce": "Salesforce",
	"tableau":    "Tableau",
	// Soft skills
	"communication":          "Communication",
	"leadership":             "Leadership",
	"teamwork":               "Teamwork",
	"collaboration":          "Collaboration",
	"stakeholder management": "Stakeholder Management",
	"mentorship":             "Mentorship",
	"problem solving":        "Problem Solving",
	"problem-solving":        "Problem Solving",
	"critical thinking":      "Critical Thinking",
	// Certifications / domain terms
	"aws certified": "AWS Certified",
	"pmp":           "PMP",
	"hipaa":         "HIPAA",
	"fintech":       "Fintech",
	"healthcare":    "Healthcare",
}

// blocklist contains lowercase terms that carry no matching signal at all —
// generic office tools, generic OS names, and bare acronyms too broad to be
// meaningful on their own (e.g. "api" alone, vs. a specific tool/platform
// name). Methodologies, non-dev tools, soft skills, and certification/domain
// terms are deliberately NOT blocked here — see `canonical` above — since
// broadened job-keyword extraction now needs them for resume matching.
var blocklist = map[string]bool{
	// Generic office tools
	"microsoft office": true,
	"excel":            true,
	"word":             true,
	"powerpoint":       true,
	"google docs":      true,
	"google sheets":    true,
	// Generic OS (not specific enough)
	"windows": true,
	"macos":   true,
	"unix":    true,
	// Other noise
	"api":             true,
	"apis":            true,
	"sdk":             true,
	"json":            true,
	"xml":             true,
	"html":            true,
	"css":             true,
	"sql":             true,
	"nosql":           true,
	"version control": true,
}

// Clean normalizes and filters a raw tech_stack slice from LLM extraction.
// It returns deduplicated canonical tech names with noise removed.
func Clean(raw []string) []string {
	seen := make(map[string]bool, len(raw))
	result := make([]string, 0, len(raw))

	for _, t := range raw {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}

		key := strings.ToLower(t)

		if blocklist[key] {
			continue
		}

		// Apply canonical mapping if one exists; otherwise keep original casing.
		if mapped, ok := canonical[key]; ok {
			t = mapped
			key = strings.ToLower(mapped)
		}

		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, t)
	}

	return result
}
