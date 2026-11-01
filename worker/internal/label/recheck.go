package label

import (
	"fmt"
	"math/rand/v2"
	"sort"
)

// Recheck writes a further pass over a dataset: the chosen items in a new seeded order. It's
// a dataset of its own with its own label log, so the first answers are never on screen.
// Refresh, if set, changes the chosen items before they're saved.
func Recheck(root, dataset, recheck string, ids []string, seed string, refresh func([]Item) error) error {
	path, _ := Paths(root, dataset)
	items, err := loadItems(path)
	if err != nil {
		return fmt.Errorf("load items: %w", err)
	}
	byID := make(map[string]Item, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	chosen := make([]Item, 0, len(ids))
	for _, id := range ids {
		item, ok := byID[id]
		if !ok {
			return fmt.Errorf("%s has no item %q", dataset, id)
		}
		chosen = append(chosen, item)
	}
	if refresh != nil {
		if err := refresh(chosen); err != nil {
			return err
		}
	}
	mix(chosen, seed)
	return WriteItems(root, recheck, chosen)
}

// Agreement compares two passes over the same items, for one question.
type Agreement struct {
	Question  string
	Pairs     int
	Same      int
	Kappa     float64
	Low, High float64 // 95% bootstrap interval
	Table     map[[2]string]int
}

// finals replays a label log into its final answers, so an undone answer doesn't count.
func finals(path string) (map[key]string, error) {
	s := &Session{answers: map[key]string{}}
	if err := s.replay(path); err != nil {
		return nil, err
	}
	return s.answers, nil
}

// Compare scores the recheck against the first pass, per question. It refuses until every
// recheck item is finished, because a score seen part way through would no longer be blind.
// A conditional question counts only for items where both passes asked it.
func Compare(root, dataset, recheck string) ([]Agreement, error) {
	questions, ok := Questions[recheck]
	if !ok {
		return nil, fmt.Errorf("unknown dataset %q", recheck)
	}
	path, secondLog := Paths(root, recheck)
	items, err := loadItems(path)
	if err != nil {
		return nil, fmt.Errorf("load items: %w", err)
	}
	second, err := finals(secondLog)
	if err != nil {
		return nil, err
	}
	state := &Session{questions: questions, answers: second}
	for _, item := range items {
		if state.pending(item) != nil {
			return nil, fmt.Errorf("%s isn't finished", recheck)
		}
	}
	_, firstLog := Paths(root, dataset)
	first, err := finals(firstLog)
	if err != nil {
		return nil, err
	}
	var out []Agreement
	for _, q := range questions {
		pairs := make([][2]string, 0, len(items))
		for _, item := range items {
			k := key{item.ID, q.ID}
			a, inFirst := first[k]
			b, inSecond := second[k]
			if inFirst && inSecond {
				pairs = append(pairs, [2]string{a, b})
			} else if q.OnlyIf == "" {
				return nil, fmt.Errorf("%s has no answer for an item in %s", dataset, recheck)
			}
		}
		out = append(out, agreementOn(q.ID, pairs))
	}
	return out, nil
}

// AuditScore compares the human's audit answers with the model's, per question: the first
// value of each pair is the model's. Like Compare it waits for a finished pass. A question
// counts only where both answered it, so the level question drops out of out-of-scope items.
func AuditScore(root, dataset string) ([]Agreement, error) {
	questions, ok := Questions[dataset]
	if !ok {
		return nil, fmt.Errorf("unknown dataset %q", dataset)
	}
	path, labels := Paths(root, dataset)
	items, err := loadItems(path)
	if err != nil {
		return nil, fmt.Errorf("load items: %w", err)
	}
	human, err := finals(labels)
	if err != nil {
		return nil, err
	}
	state := &Session{questions: questions, answers: human}
	for _, item := range items {
		if state.pending(item) != nil {
			return nil, fmt.Errorf("%s isn't finished", dataset)
		}
	}
	var out []Agreement
	for _, q := range questions {
		if q.ID == seen.ID {
			continue
		}
		var pairs [][2]string
		for _, item := range items {
			model, byModel := item.Model[q.ID]
			answer, byHuman := human[key{item.ID, q.ID}]
			if byModel && byHuman {
				pairs = append(pairs, [2]string{model, answer})
			}
		}
		out = append(out, agreementOn(q.ID, pairs))
	}
	return out, nil
}

func agreementOn(question string, pairs [][2]string) Agreement {
	out := Agreement{Question: question, Pairs: len(pairs), Kappa: kappa(pairs), Table: map[[2]string]int{}}
	for _, p := range pairs {
		out.Table[p]++
		if p[0] == p[1] {
			out.Same++
		}
	}
	out.Low, out.High = interval(pairs)
	return out
}

// kappa is Cohen's kappa over all answer values, unsure included as its own value. If both
// passes used one value throughout, chance explains everything; that counts as 1.
func kappa(pairs [][2]string) float64 {
	if len(pairs) == 0 {
		return 0
	}
	n := float64(len(pairs))
	same := 0.0
	left, right := map[string]float64{}, map[string]float64{}
	for _, p := range pairs {
		if p[0] == p[1] {
			same++
		}
		left[p[0]]++
		right[p[1]]++
	}
	chance := 0.0
	for value, count := range left {
		chance += count / n * right[value] / n
	}
	if chance == 1 {
		return 1
	}
	return (same/n - chance) / (1 - chance)
}

// interval is a percentile bootstrap over items, with a fixed seed so a report reruns exactly.
func interval(pairs [][2]string) (float64, float64) {
	const rounds = 2000
	if len(pairs) == 0 {
		return 0, 0
	}
	random := rand.New(rand.NewPCG(1, 2))
	scores := make([]float64, rounds)
	sample := make([][2]string, len(pairs))
	for r := range scores {
		for i := range sample {
			sample[i] = pairs[random.IntN(len(pairs))]
		}
		scores[r] = kappa(sample)
	}
	sort.Float64s(scores)
	return scores[rounds*25/1000], scores[rounds*975/1000]
}
