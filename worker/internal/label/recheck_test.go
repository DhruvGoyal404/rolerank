package label

import (
	"math"
	"strings"
	"testing"
)

func pairItems(ids ...string) []Item {
	items := make([]Item, 0, len(ids))
	for _, id := range ids {
		items = append(items, Item{ID: id, A: Side{ID: id + "a"}, B: &Side{ID: id + "b"}})
	}
	return items
}

func answer(t *testing.T, root, dataset string, keys ...string) {
	t.Helper()
	s, err := Open(root, dataset)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, k := range keys {
		if k == "undo" {
			if err := s.Undo(); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := s.Press(k); err != nil {
			t.Fatal(err)
		}
	}
}

// Textbook case: 20 yes-yes, 5 yes-no, 10 no-yes, 15 no-no gives 0.7 observed, 0.5 by chance.
func TestKappaKnownValue(t *testing.T) {
	var pairs [][2]string
	for value, count := range map[[2]string]int{
		{"yes", "yes"}: 20, {"yes", "no"}: 5, {"no", "yes"}: 10, {"no", "no"}: 15,
	} {
		for range count {
			pairs = append(pairs, value)
		}
	}
	if got := kappa(pairs); math.Abs(got-0.4) > 1e-9 {
		t.Fatalf("got %v", got)
	}
}

func TestKappaWhenOnlyOneValueWasUsed(t *testing.T) {
	if got := kappa([][2]string{{"yes", "yes"}, {"yes", "yes"}}); got != 1 {
		t.Fatalf("got %v", got)
	}
}

func TestRecheckIsItsOwnDatasetInANewOrder(t *testing.T) {
	root := t.TempDir()
	ids := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	if err := WriteItems(root, "d5", pairItems(ids...)); err != nil {
		t.Fatal(err)
	}
	answer(t, root, "d5", "y", "y", "y", "y", "y", "y", "y", "y")
	if err := Recheck(root, "d5", "d5-recheck", ids[:6], "seed", nil); err != nil {
		t.Fatal(err)
	}
	path, _ := Paths(root, "d5-recheck")
	got, err := loadItems(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 6 {
		t.Fatalf("%d items", len(got))
	}
	moved := false
	for i := range got {
		moved = moved || got[i].ID != ids[i]
	}
	if !moved {
		t.Fatal("the order didn't change")
	}
	s, err := Open(root, "d5-recheck")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if screen := s.Current(); screen.Done != 0 || screen.Finished {
		t.Fatalf("first answers leaked into the second pass: %+v", screen)
	}
}

func TestRecheckRefusesAnUnknownItem(t *testing.T) {
	root := t.TempDir()
	if err := WriteItems(root, "d5", pairItems("a")); err != nil {
		t.Fatal(err)
	}
	if err := Recheck(root, "d5", "d5-recheck", []string{"nope"}, "seed", nil); err == nil {
		t.Fatal("want an error")
	}
}

func TestCompareRefusesAnUnfinishedPass(t *testing.T) {
	root := t.TempDir()
	if err := WriteItems(root, "d5", pairItems("a", "b")); err != nil {
		t.Fatal(err)
	}
	answer(t, root, "d5", "y", "n")
	if err := Recheck(root, "d5", "d5-recheck", []string{"a", "b"}, "seed", nil); err != nil {
		t.Fatal(err)
	}
	answer(t, root, "d5-recheck", "y")
	if _, err := Compare(root, "d5", "d5-recheck"); err == nil {
		t.Fatal("a score part way through wouldn't be blind")
	}
}

func TestCompareUsesFinalAnswers(t *testing.T) {
	root := t.TempDir()
	if err := WriteItems(root, "d5", pairItems("a", "b")); err != nil {
		t.Fatal(err)
	}
	answer(t, root, "d5", "n", "undo", "y", "y")
	if err := WriteItems(root, "d5-recheck", pairItems("a", "b")); err != nil {
		t.Fatal(err)
	}
	answer(t, root, "d5-recheck", "y", "n")
	got, err := Compare(root, "d5", "d5-recheck")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Pairs != 2 || got[0].Same != 1 || got[0].Table[[2]string{"yes", "no"}] != 1 {
		t.Fatalf("%+v", got)
	}
}

// Each question has its own score, and level counts only where both passes asked it.
func TestCompareScoresEachQuestion(t *testing.T) {
	root := t.TempDir()
	items := []Item{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	if err := WriteItems(root, "d6", items); err != nil {
		t.Fatal(err)
	}
	answer(t, root, "d6", "y", "e", "y", "y", "m", "n", "n", "n")
	if err := WriteItems(root, "d6-recheck", items); err != nil {
		t.Fatal(err)
	}
	answer(t, root, "d6-recheck", "y", "e", "y", "n", "n", "y", "i", "n")
	got, err := Compare(root, "d6", "d6-recheck")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("%+v", got)
	}
	want := map[string][2]int{"engineering": {3, 1}, "level": {1, 1}, "india": {3, 3}}
	for _, a := range got {
		if w := want[a.Question]; a.Pairs != w[0] || a.Same != w[1] {
			t.Errorf("%s: %+v", a.Question, a)
		}
	}
}

func TestCompareRefusesAConditionalQuestionLeftOpen(t *testing.T) {
	root := t.TempDir()
	items := []Item{{ID: "a"}}
	if err := WriteItems(root, "d6", items); err != nil {
		t.Fatal(err)
	}
	answer(t, root, "d6", "y", "e", "y")
	if err := WriteItems(root, "d6-recheck", items); err != nil {
		t.Fatal(err)
	}
	answer(t, root, "d6-recheck", "y", "e")
	if _, err := Compare(root, "d6", "d6-recheck"); err == nil {
		t.Fatal("want an error")
	}
}

func TestRefreshRunsBeforeItemsAreSaved(t *testing.T) {
	root := t.TempDir()
	if err := WriteItems(root, "d5", pairItems("a")); err != nil {
		t.Fatal(err)
	}
	refresh := func(items []Item) error {
		return applyBodies(items, map[string]string{"aa": "whole text a", "ab": "whole text b"})
	}
	if err := Recheck(root, "d5", "d5-redo", []string{"a"}, "seed", refresh); err != nil {
		t.Fatal(err)
	}
	path, _ := Paths(root, "d5-redo")
	got, err := loadItems(path)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].A.Body != "whole text a" || got[0].B.Body != "whole text b" {
		t.Fatalf("%+v", got[0])
	}
}

func TestApplyBodiesRefusesAMissingPosting(t *testing.T) {
	if err := applyBodies(pairItems("a"), map[string]string{"aa": "x"}); err == nil {
		t.Fatal("a pair can't go out with one side still cut short")
	}
}

// The rule sits in the question, so every pass shows it: the job rule on pairs, the four
// scope rules on every D6 question.
func TestPagesCarryTheRules(t *testing.T) {
	for _, dataset := range []string{"d5", "d5-recheck", "d5-redo", "d5-redo-2", "d5-audit", "d5-merged-audit"} {
		q := Questions[dataset][0]
		if !strings.Contains(q.Rule, "same location") || !strings.Contains(q.Rule, "different city") {
			t.Errorf("%s: %q", dataset, q.Rule)
		}
	}
	for _, dataset := range []string{"d6", "d6-recheck", "d6-audit"} {
		for _, q := range Questions[dataset] {
			if q.ID != seen.ID && (!strings.Contains(q.Rule, "VLSI") || !strings.Contains(q.Rule, "region not stated")) {
				t.Errorf("%s %s: %q", dataset, q.ID, q.Rule)
			}
		}
	}
}

func auditItems() []Item {
	items := pairItems("p1", "p2", "p3", "p4")
	for i, same := range []string{"yes", "yes", "no", "no"} {
		items[i].Model = map[string]string{"same": same, "reason": "MODEL-REASON", "conf": "high"}
	}
	return items
}

// The model's answer stays off the page until the human has answered the item.
func TestAuditRevealsTheModelOnlyAfterTheAnswer(t *testing.T) {
	root := t.TempDir()
	if err := WriteItems(root, "d5-audit", auditItems()); err != nil {
		t.Fatal(err)
	}
	s := open(t, root, "d5-audit")
	if screen := s.Current(); screen.Model != nil || screen.Question.ID != "same" {
		t.Fatalf("before answering: %+v", screen)
	}
	press(t, s, "n")
	screen := s.Current()
	if screen.Question.ID != seen.ID || screen.Model["reason"] != "MODEL-REASON" || screen.A.ID != "p1a" {
		t.Fatalf("after answering: %+v", screen)
	}
	press(t, s, "c")
	if screen := s.Current(); screen.Model != nil || screen.A.ID != "p2a" {
		t.Fatalf("next item: %+v", screen)
	}
}

func TestAuditScoreComparesHumanWithModel(t *testing.T) {
	root := t.TempDir()
	if err := WriteItems(root, "d5-audit", auditItems()); err != nil {
		t.Fatal(err)
	}
	answer(t, root, "d5-audit", "y", "c", "n", "c", "n")
	if _, err := AuditScore(root, "d5-audit"); err == nil {
		t.Fatal("scored an unfinished audit")
	}
	answer(t, root, "d5-audit", "c", "n", "c")
	got, err := AuditScore(root, "d5-audit")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Question != "same" || got[0].Pairs != 4 || got[0].Same != 3 ||
		got[0].Table[[2]string{"yes", "no"}] != 1 {
		t.Fatalf("%+v", got)
	}
}
