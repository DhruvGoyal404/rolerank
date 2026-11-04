package skills

import (
	"slices"
	"testing"
)

func TestDefaultTaxonomyParses(t *testing.T) {
	if n := len(Default.Names()); n < 400 {
		t.Fatalf("only %d skills", n)
	}
}

func TestParseRejectsAnAliasWithTwoMeanings(t *testing.T) {
	if _, err := Parse("A | x | shared\nB | x | shared\n"); err == nil {
		t.Fatal("want an error")
	}
}

func TestFind(t *testing.T) {
	tests := []struct {
		text string
		want []string
	}{
		{"Experience with React.js, Node.js. and k8s", []string{"React", "Node.js", "Kubernetes"}},
		{"We use Go and gRPC", []string{"Go", "gRPC"}},
		{"go to the office", nil},
		{"C++ and C# on .NET", []string{"C++", "C#", ".NET"}},
		{"CI/CD with GitHub Actions", []string{"CI/CD", "GitHub Actions"}},
		{"react-native and React Native apps", []string{"React Native"}},
		{"Spring Boot microservices", []string{"Spring Boot", "Microservices"}},
		{"Spring 2027 internship in Phoenix", nil},
		{"express your interest; excel at teamwork", nil},
		{"Strong SQL; PostgreSQL or MySQL", []string{"SQL", "PostgreSQL", "MySQL"}},
		{"Python, R and Excel", []string{"Python", "R", "Excel"}},
		{"C/C++ and C, not (C), P&C, C-level or Series C", []string{"C", "C++"}},
		{"networking events and documentation", nil},
	}
	for _, tt := range tests {
		if got := Default.Find(tt.text); !slices.Equal(got, tt.want) {
			t.Errorf("%q: got %v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestExtractSplitsRequiredFromNiceToHave(t *testing.T) {
	body := "About the role\nYou will build services in Go.\nRequirements\n- 2+ years of Python\n- PostgreSQL\n" +
		"Nice to have\n- Kubernetes\n- Python\nBenefits\nThis role uses Terraform, which is a plus."
	req, nice := Default.Extract(body)
	if !slices.Equal(req, []string{"Go", "PostgreSQL", "Python"}) {
		t.Errorf("required %v", req)
	}
	if !slices.Equal(nice, []string{"Kubernetes", "Terraform"}) {
		t.Errorf("nice %v", nice)
	}
}
