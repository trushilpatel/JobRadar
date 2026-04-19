// Package techtag normalizes and filters raw tech_stack strings extracted by the LLM.
// It maps variant spellings to canonical names and drops non-tech terms.
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
}

// blocklist contains lowercase terms that are not valid tech stack entries.
// These are methodologies, soft skills, patterns, and generic tools.
var blocklist = map[string]bool{
	// Methodologies
	"agile":     true,
	"scrum":     true,
	"kanban":    true,
	"waterfall": true,
	"lean":      true,
	// Architecture patterns (too generic)
	"rest":                   true,
	"restful":                true,
	"rest api":               true,
	"rest apis":              true,
	"soap":                   true,
	"mvc":                    true,
	"mvvm":                   true,
	"microservices":          true,
	"serverless":             true,
	"solid":                  true,
	"oop":                    true,
	"object-oriented":        true,
	"functional programming": true,
	"ci/cd":                  true,
	"ci cd":                  true,
	"devops":                 true,
	// Soft skills
	"communication":     true,
	"leadership":        true,
	"teamwork":          true,
	"collaboration":     true,
	"problem solving":   true,
	"problem-solving":   true,
	"critical thinking": true,
	"verbal":            true,
	"written":           true,
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
