package label

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const secretText = "PLANTED-POSTING-TEXT"

func d6Items() []Item {
	var items []Item
	for _, id := range []string{"one", "two", "three"} {
		items = append(items, Item{
			ID: id, Stratum: "india",
			A:      Side{ID: id, Title: "Engineer", Body: secretText},
			System: map[string]string{"level": "SYSTEM-GUESS"},
		})
	}
	return items
}

func open(t *testing.T, root, dataset string) *Session {
	t.Helper()
	s, err := Open(root, dataset)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func setup(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := WriteItems(root, "d6", d6Items()); err != nil {
		t.Fatal(err)
	}
	return root
}

func press(t *testing.T, s *Session, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if err := s.Press(k); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOneItemAtATimeWithProgress(t *testing.T) {
	s := open(t, setup(t), "d6")
	screen := s.Current()
	if screen.Total != 3 || screen.Done != 0 || screen.A.ID != "one" || screen.Question.ID != "engineering" {
		t.Fatalf("%+v", screen)
	}
	press(t, s, "y", "e", "y")
	screen = s.Current()
	if screen.Done != 1 || screen.A.ID != "two" {
		t.Fatalf("%+v", screen)
	}
}

func TestOutOfScopeSkipsTheLevelQuestion(t *testing.T) {
	s := open(t, setup(t), "d6")
	press(t, s, "n")
	if got := s.Current().Question.ID; got != "india" {
		t.Fatalf("asked %s", got)
	}
}

func TestUndoStepsBackAcrossItems(t *testing.T) {
	s := open(t, setup(t), "d6")
	press(t, s, "y", "e", "y", "n")
	for _, want := range []string{"engineering", "india"} {
		if err := s.Undo(); err != nil {
			t.Fatal(err)
		}
		if got := s.Current().Question.ID; got != want {
			t.Fatalf("after undo at %s, want %s", got, want)
		}
	}
	if s.Current().A.ID != "one" {
		t.Fatal("undo must return to the earlier item")
	}
	empty := open(t, setup(t), "d6")
	if err := empty.Undo(); err != nil {
		t.Fatal("undo with nothing to undo must be harmless")
	}
}

func TestResumesExactlyWhereItStopped(t *testing.T) {
	root := setup(t)
	s, err := Open(root, "d6")
	if err != nil {
		t.Fatal(err)
	}
	press(t, s, "y", "e", "y", "y")
	if err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	press(t, s, "n")
	_ = s.Close()

	again := open(t, root, "d6")
	screen := again.Current()
	if screen.Done != 1 || screen.A.ID != "two" || screen.Question.ID != "india" {
		t.Fatalf("%+v", screen)
	}
	if err := again.Undo(); err != nil {
		t.Fatal(err)
	}
	if got := again.Current().Question.ID; got != "engineering" {
		t.Fatalf("undo after resume landed on %s", got)
	}
}

func TestSurvivesALineCutShortByACrash(t *testing.T) {
	root := setup(t)
	s, err := Open(root, "d6")
	if err != nil {
		t.Fatal(err)
	}
	press(t, s, "y")
	_ = s.Close()
	_, labels := Paths(root, "d6")
	file, err := os.OpenFile(labels, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.WriteString(`{"item":"one","que`)
	_ = file.Close()
	if got := open(t, root, "d6").Current().Question.ID; got != "level" {
		t.Fatalf("resumed at %s", got)
	}
}

func TestSkipCountsAsAnsweredButStaysDistinct(t *testing.T) {
	root := setup(t)
	s := open(t, root, "d6")
	press(t, s, "s", "s", "s")
	if s.Current().Done != 1 {
		t.Fatal("unsure must move on")
	}
	_, labels := Paths(root, "d6")
	data, _ := os.ReadFile(labels)
	if strings.Count(string(data), `"unsure"`) != 3 {
		t.Fatalf("%s", data)
	}
}

func TestMeaninglessKeysDoNothing(t *testing.T) {
	s := open(t, setup(t), "d6")
	press(t, s, "x", "9", "")
	if s.Current().Question.ID != "engineering" {
		t.Fatal("a stray key must not answer")
	}
}

func TestPostingTextNeverEntersTheLabelFile(t *testing.T) {
	root := setup(t)
	s := open(t, root, "d6")
	press(t, s, "y", "e", "y", "n", "y")
	_, labels := Paths(root, "d6")
	data, err := os.ReadFile(labels)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), secretText) || strings.Contains(string(data), "Engineer") {
		t.Fatalf("label file holds posting text: %s", data)
	}
	if !strings.Contains(filepath.ToSlash(labels), "evals/labels/") {
		t.Fatalf("labels went to %s", labels)
	}
}

func TestSelectionCantReplaceLabelledWork(t *testing.T) {
	root := setup(t)
	s := open(t, root, "d6")
	press(t, s, "y")
	if err := WriteItems(root, "d6", d6Items()); err == nil {
		t.Fatal("a new selection would orphan the labels")
	}
}

func TestPageIsBlindAndWritesNeedTheHeader(t *testing.T) {
	root := setup(t)
	s := open(t, root, "d6")
	server := httptest.NewServer(Handler(s))
	defer server.Close()

	resp, err := http.Get(server.URL + "/state")
	if err != nil {
		t.Fatal(err)
	}
	var body strings.Builder
	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	body.Write(buf[:n])
	resp.Body.Close()
	if strings.Contains(body.String(), "SYSTEM-GUESS") {
		t.Fatal("the system's guess reached the page")
	}

	plain, err := http.Post(server.URL+"/press?key=y", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	plain.Body.Close()
	if plain.StatusCode != http.StatusForbidden || s.Current().Question.ID != "engineering" {
		t.Fatalf("a write without the header got %d", plain.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodPost, server.URL+"/press?key=y", nil)
	req.Header.Set("X-Label", "1")
	ok, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	ok.Body.Close()
	if ok.StatusCode != http.StatusOK || s.Current().Question.ID != "level" {
		t.Fatalf("a proper write got %d", ok.StatusCode)
	}
}

func TestPageRendersTextNotHTML(t *testing.T) {
	text := string(page)
	if strings.Contains(text, "innerHTML") || strings.Contains(text, "document.write") {
		t.Fatal("posting text must render as text")
	}
}
