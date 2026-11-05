// Command skillfixture writes the cases the TypeScript port of the skill matcher must agree on
// (api/src/ai/skills.ts): text in, the Go matcher's Find and Extract out. Run it after any change
// to the taxonomy or matcher:
//
//	go run ./cmd/skillfixture > ../api/src/ai/testdata/skills.json
package main

import (
	"encoding/json"
	"os"

	"github.com/DhruvGoyal404/rolerank/worker/internal/skills"
)

var finds = []string{
	"Experience with React.js, Node.js. and k8s",
	"We use Go and gRPC",
	"go to the office",
	"C++ and C# on .NET",
	"CI/CD with GitHub Actions",
	"react-native and React Native apps",
	"Spring Boot microservices",
	"Spring 2027 internship in Phoenix",
	"express your interest; excel at teamwork",
	"Strong SQL; PostgreSQL or MySQL",
	"Python, R and Excel",
	"C/C++ and C, not (C), P&C, C-level or Series C",
	"networking events and documentation",
	"Built REST APIs in FastAPI, deployed on AWS Lambda with Terraform",
	"PyTorch, TensorFlow, scikit-learn; LLMs & RAG pipelines on Kubernetes",
	"Unity Catalog on Databricks, Spark and Airflow",
	"Docker containers and Git version control",
	"Senior Software Engineer (Go/Kubernetes) - Bengaluru",
	"B.Tech Computer Science, CGPA 8.4; Data Structures and Algorithms",
	"Selenium, Cypress and Playwright for test automation; JMeter load testing",
}

var extracts = []string{
	"About the role\nYou will build services in Go.\nRequirements\n- 2+ years of Python\n- PostgreSQL\nNice to have\n- Kubernetes\n- Python\nBenefits\nThis role uses Terraform, which is a plus.",
	"What you'll do\nShip React and TypeScript features.\nQualifications\nJavaScript, HTML, CSS\nBonus: GraphQL, Next.js",
	"Must have: Java, Spring Boot, Kafka\nGood to have: AWS, Docker\nWe use Jira and Confluence.",
}

func main() {
	type find struct {
		Text string   `json:"text"`
		Want []string `json:"want"`
	}
	type extract struct {
		Body     string   `json:"body"`
		Required []string `json:"required"`
		Nice     []string `json:"nice"`
	}
	out := struct {
		Find    []find    `json:"find"`
		Extract []extract `json:"extract"`
	}{}
	for _, t := range finds {
		out.Find = append(out.Find, find{t, append([]string{}, skills.Default.Find(t)...)})
	}
	for _, b := range extracts {
		req, nice := skills.Default.Extract(b)
		out.Extract = append(out.Extract, extract{b, append([]string{}, req...), append([]string{}, nice...)})
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", " ")
	_ = enc.Encode(out)
}
