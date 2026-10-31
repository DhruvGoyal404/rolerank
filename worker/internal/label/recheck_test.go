package label

import (
	"math"
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
	if err := Recheck(root, "d5", "d5-recheck", ids[:6], "seed"); err != nil {
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
	if err := Recheck(root, "d5", "d5-recheck", []string{"nope"}, "seed"); err == nil {
		t.Fatal("want an error")
	}
}

func TestCompareRefusesAnUnfinishedPass(t *testing.T) {
	root := t.TempDir()
	if err := WriteItems(root, "d5", pairItems("a", "b")); err != nil {
		t.Fatal(err)
	}
	answer(t, root, "d5", "y", "n")
	if err := Recheck(root, "d5", "d5-recheck", []string{"a", "b"}, "seed"); err != nil {
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
	if got.Pairs != 2 || got.Same != 1 || got.Table[[2]string{"yes", "no"}] != 1 {
		t.Fatalf("%+v", got)
	}
}
