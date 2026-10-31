// Package label collects the hand labels for the D5 and D6 evals. Item text is real posting
// text, so it stays in the gitignored evals/private folder. Labels hold only IDs and
// answers, so they can live in the repo. Labelling is blind: the system's own guess is
// stored for scoring, and never shown.
package label

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Side is one posting as the labeller sees it.
type Side struct {
	ID       string `json:"id"`
	Source   string `json:"source"`
	Company  string `json:"company"`
	Title    string `json:"title"`
	Location string `json:"location"`
	Body     string `json:"body"`
	URL      string `json:"url"`
}

// Item is one screen: a single posting for D6, a pair for D5.
type Item struct {
	ID      string            `json:"id"`
	Stratum string            `json:"stratum"`
	A       Side              `json:"a"`
	B       *Side             `json:"b,omitempty"`
	System  map[string]string `json:"system,omitempty"` // for scoring, never sent to the page
}

// Choice is one key and what it means.
type Choice struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Label string `json:"label"`
}

// Question is asked of each item in a dataset.
type Question struct {
	ID       string   `json:"id"`
	Text     string   `json:"text"`
	Help     string   `json:"help"`
	Rule     string   `json:"rule,omitempty"` // shown at the top of the page, above the postings
	Choices  []Choice `json:"choices"`
	OnlyIf   string   `json:"-"` // question ID that must not have the value in SkipWhen
	SkipWhen string   `json:"-"`
}

var unsure = Choice{"s", "unsure", "skip / unsure"}

var d5Questions = []Question{{
	ID:   "same",
	Text: "Are these the same job?",
	Rule: "Same employer + same role + same location (or same remote scope) = same job. A different city is a different job.",
	Help: "A re-listing of one posting on another site is the same job.",
	Choices: []Choice{
		{"y", "yes", "same job"}, {"n", "no", "different"}, unsure,
	},
}}

var d6Questions = []Question{
	{
		ID:   "engineering",
		Text: "Is this a software or engineering role in our scope?",
		Help: "Yes for software, data, ML, QA, DevOps and security engineering. No for sales engineers, recruiters, and civil or mechanical engineering.",
		Choices: []Choice{
			{"y", "yes", "in scope"}, {"n", "no", "out of scope"}, unsure,
		},
	},
	{
		ID:       "level",
		Text:     "What level is this role?",
		Help:     "Entry means open to freshers or up to about 1 year. Judge by what the posting asks for, not only the title.",
		OnlyIf:   "engineering",
		SkipWhen: "no",
		Choices: []Choice{
			{"i", "intern", "intern"}, {"e", "entry", "entry, 0-1 yr"}, {"m", "mid", "mid, 2-4 yr"},
			{"r", "senior", "senior, 5+ yr"}, {"k", "unknown", "can't tell"}, unsure,
		},
	},
	{
		ID:   "india",
		Text: "Can someone living in India take this job?",
		Help: "Yes if it's based in India, or remote and genuinely open to India. A remote role limited to another region is no.",
		Choices: []Choice{
			{"y", "yes", "yes"}, {"n", "no", "no"}, unsure,
		},
	},
}

// Questions lists what each dataset asks. A recheck or redo asks what its dataset asked.
var Questions = map[string][]Question{
	"d5":         d5Questions,
	"d5-recheck": d5Questions,
	"d5-redo":    d5Questions,
	"d5-redo-2":  d5Questions,
	"d6":         d6Questions,
	"d6-recheck": d6Questions,
}

// Answer is one line of the label log. Undo appends a line too: the log is never rewritten,
// so no crash can lose an earlier answer.
type Answer struct {
	Item     string `json:"item"`
	Question string `json:"question"`
	Value    string `json:"value,omitempty"`
	Undo     bool   `json:"undo,omitempty"`
	At       string `json:"at"`
}

// Paths are where a dataset's files live.
func Paths(root, dataset string) (items, labels string) {
	return filepath.Join(root, "evals", "private", dataset+"-items.jsonl"),
		filepath.Join(root, "evals", "labels", dataset+".jsonl")
}

// WriteItems saves a selection. It refuses to replace one that already has labels, because
// new items would orphan them.
func WriteItems(root, dataset string, items []Item) error {
	itemsPath, labelsPath := Paths(root, dataset)
	if info, err := os.Stat(labelsPath); err == nil && info.Size() > 0 {
		return fmt.Errorf("%s already has labels; move them aside first", dataset)
	}
	if err := os.MkdirAll(filepath.Dir(itemsPath), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(itemsPath+".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	out := bufio.NewWriter(file)
	encoder := json.NewEncoder(out)
	for _, item := range items {
		if err := encoder.Encode(item); err != nil {
			file.Close()
			return err
		}
	}
	if err := out.Flush(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(itemsPath+".tmp", itemsPath)
}

func loadItems(path string) ([]Item, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var items []Item
	lines := bufio.NewScanner(file)
	lines.Buffer(make([]byte, 1<<20), 16<<20)
	for lines.Scan() {
		var item Item
		if err := json.Unmarshal(lines.Bytes(), &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, lines.Err()
}

type key struct{ item, question string }

// Session is one labelling sitting. It isn't safe for concurrent use; the server locks.
type Session struct {
	dataset   string
	items     []Item
	questions []Question
	answers   map[key]string
	history   []key
	log       *os.File
	now       func() time.Time
}

// Open loads a dataset and replays its label log, so work resumes exactly where it stopped.
func Open(root, dataset string) (*Session, error) {
	questions, ok := Questions[dataset]
	if !ok {
		return nil, fmt.Errorf("unknown dataset %q", dataset)
	}
	itemsPath, labelsPath := Paths(root, dataset)
	items, err := loadItems(itemsPath)
	if err != nil {
		return nil, fmt.Errorf("load items: %w", err)
	}
	s := &Session{
		dataset: dataset, items: items, questions: questions,
		answers: map[key]string{}, now: time.Now,
	}
	if err := s.replay(labelsPath); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(labelsPath), 0o755); err != nil {
		return nil, err
	}
	s.log, err = os.OpenFile(labelsPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Session) replay(path string) error {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	lines := bufio.NewScanner(file)
	for lines.Scan() {
		var a Answer
		if err := json.Unmarshal(lines.Bytes(), &a); err != nil {
			continue // a line cut short by a crash; everything before it is intact
		}
		k := key{a.Item, a.Question}
		if a.Undo {
			delete(s.answers, k)
			if n := len(s.history); n > 0 && s.history[n-1] == k {
				s.history = s.history[:n-1]
			}
			continue
		}
		s.answers[k] = a.Value
		s.history = append(s.history, k)
	}
	return lines.Err()
}

// Close releases the log.
func (s *Session) Close() error {
	return s.log.Close()
}

func (s *Session) write(a Answer) error {
	a.At = s.now().UTC().Format(time.RFC3339Nano) // whole seconds can't show a 1.5 s answer
	data, err := json.Marshal(a)
	if err != nil {
		return err
	}
	if _, err := s.log.Write(append(data, '\n')); err != nil {
		return err
	}
	return s.log.Sync()
}

func (s *Session) pending(item Item) *Question {
	for i := range s.questions {
		q := &s.questions[i]
		if q.OnlyIf != "" && s.answers[key{item.ID, q.OnlyIf}] == q.SkipWhen {
			continue
		}
		if _, done := s.answers[key{item.ID, q.ID}]; !done {
			return q
		}
	}
	return nil
}

// Screen is what the page shows. It never carries the system's guess.
type Screen struct {
	Dataset  string    `json:"dataset"`
	Done     int       `json:"done"`
	Total    int       `json:"total"`
	Finished bool      `json:"finished"`
	A        *Side     `json:"a,omitempty"`
	B        *Side     `json:"b,omitempty"`
	Question *Question `json:"question,omitempty"`
}

// Current returns the first unanswered question.
func (s *Session) Current() Screen {
	screen := Screen{Dataset: s.dataset, Total: len(s.items), Finished: true}
	found := false
	for i := range s.items {
		item := s.items[i]
		q := s.pending(item)
		if q == nil {
			screen.Done++
			continue
		}
		if !found {
			found = true
			a := item.A
			screen.A, screen.B, screen.Question, screen.Finished = &a, item.B, q, false
		}
	}
	return screen
}

// Press records a key for the current question. A key that means nothing is ignored.
func (s *Session) Press(pressed string) error {
	for i := range s.items {
		q := s.pending(s.items[i])
		if q == nil {
			continue
		}
		for _, c := range q.Choices {
			if c.Key != pressed {
				continue
			}
			k := key{s.items[i].ID, q.ID}
			if err := s.write(Answer{Item: k.item, Question: k.question, Value: c.Value}); err != nil {
				return err
			}
			s.answers[k] = c.Value
			s.history = append(s.history, k)
			return nil
		}
		return nil
	}
	return nil
}

// Undo removes the most recent answer.
func (s *Session) Undo() error {
	n := len(s.history)
	if n == 0 {
		return nil
	}
	k := s.history[n-1]
	if err := s.write(Answer{Item: k.item, Question: k.question, Undo: true}); err != nil {
		return err
	}
	delete(s.answers, k)
	s.history = s.history[:n-1]
	return nil
}
