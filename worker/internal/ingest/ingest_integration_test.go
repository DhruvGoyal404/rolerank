//go:build integration

package ingest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/valkey-io/valkey-go"

	"github.com/DhruvGoyal404/rolerank/worker/internal/fetch"
	"github.com/DhruvGoyal404/rolerank/worker/internal/limiter"
	"github.com/DhruvGoyal404/rolerank/worker/internal/source"
	"github.com/DhruvGoyal404/rolerank/worker/internal/useragent"
)

// fakeAdapter is a source served by a local test server. Its JSON is a list of IDs, with
// paging when pages > 1.
type fakeAdapter struct {
	name string
	base string
}

func (f *fakeAdapter) Name() string                    { return f.name }
func (f *fakeAdapter) Hosts() []string                 { u, _ := url.Parse(f.base); return []string{u.Host} }
func (f *fakeAdapter) Attribution() source.Attribution { return source.Attribution{Credit: "Fake"} }
func (f *fakeAdapter) FirstURL(token string) string    { return f.base + "/" + token + "?page=1" }

func (f *fakeAdapter) Parse(_ string, body []byte) (source.Page, error) {
	var data struct {
		IDs  []string `json:"ids"`
		Next string   `json:"next"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return source.Page{}, err
	}
	if data.IDs == nil {
		return source.Page{}, fmt.Errorf("no ids")
	}
	var page source.Page
	for _, id := range data.IDs {
		page.Postings = append(page.Postings, source.Posting{
			SourceJobID: id, Title: "Software Engineer " + id, Company: "Acme", Location: "Pune, India",
			Body: "<p>Build things</p>", BodyIsHTML: true, URL: "https://x.example/" + id,
		})
	}
	if data.Next != "" {
		page.NextURL = f.base + data.Next
	}
	return page, nil
}

type fakeServer struct {
	t      *testing.T
	server *httptest.Server
	mu     sync.Mutex
	pages  map[string][]string // token -> IDs for each page, comma separated
	status map[string]int
	bad    map[string]bool
	hits   []string
}

func newFakeServer(t *testing.T) *fakeServer {
	f := &fakeServer{t: t, pages: map[string][]string{}, status: map[string]int{}, bad: map[string]bool{}}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeServer) set(token string, pages ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pages[token] = pages
}

func (f *fakeServer) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.hits...)
}

func (f *fakeServer) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/robots.txt" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	token := strings.TrimPrefix(r.URL.Path, "/")
	f.hits = append(f.hits, token+"?"+r.URL.RawQuery)
	if code := f.status[token]; code != 0 {
		w.WriteHeader(code)
		return
	}
	if f.bad[token] {
		_, _ = w.Write([]byte("<html>maintenance</html>"))
		return
	}
	page := 1
	_, _ = fmt.Sscanf(r.URL.Query().Get("page"), "%d", &page)
	pages := f.pages[token]
	body := map[string]any{"ids": []string{}}
	if page <= len(pages) {
		ids := strings.Split(pages[page-1], ",")
		if pages[page-1] == "" {
			ids = []string{}
		}
		body["ids"] = ids
		if page < len(pages) {
			body["next"] = fmt.Sprintf("/%s?page=%d", token, page+1)
		}
	}
	data, _ := json.Marshal(body)
	tag := fmt.Sprintf(`"%x"`, len(data)*31+int(data[len(data)-2]))
	if len(pages) <= 1 {
		w.Header().Set("ETag", tag+strings.Join(pages, ""))
		if r.Header.Get("If-None-Match") == tag+strings.Join(pages, "") {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	_, _ = w.Write(data)
}

type harness struct {
	t       *testing.T
	store   *Store
	pool    *pgxpool.Pool
	valkey  valkey.Client
	server  *fakeServer
	adapter *fakeAdapter
	fetcher *fetch.Client
}

func randomID(t *testing.T) string {
	t.Helper()
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

func migrations(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("../../../db/corpus/migrations/*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("no migrations: %v", err)
	}
	sort.Strings(files)
	var out []string
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		up, _, _ := strings.Cut(string(data), "-- migrate:down")
		out = append(out, up)
	}
	return out
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()
	admin := os.Getenv("ROLERANK_TEST_ADMIN_URL")
	if admin == "" {
		t.Fatal("ROLERANK_TEST_ADMIN_URL is required")
	}
	name := "rrgo" + randomID(t) + "_corpus_test"
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		_ = conn.Close(context.Background())
	})
	u, err := url.Parse(admin)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	db, err := pgx.Connect(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "CREATE EXTENSION vector"); err != nil {
		t.Fatal(err)
	}
	for _, sql := range migrations(t) {
		if _, err := db.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	_ = db.Close(ctx)

	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	options, err := valkey.ParseURL(os.Getenv("ROLERANK_TEST_VALKEY_URL"))
	if err != nil {
		t.Fatal(err)
	}
	client, err := valkey.NewClient(options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)

	server := newFakeServer(t)
	adapter := &fakeAdapter{name: "fake" + randomID(t), base: server.server.URL}
	store := NewStore(pool)
	if err := store.EnsureSources(ctx, []source.Adapter{adapter}); err != nil {
		t.Fatal(err)
	}
	if err := Setup(ctx, client, adapter.Name()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = client.Do(context.Background(), client.B().Del().Key(Stream(adapter.Name())).Build()).Error()
	})
	fetcher := fetch.New(fetch.Options{
		UserAgent: useragent.String("test"), Agent: "RoleRankBot", Hosts: adapter.Hosts(),
		Limiter: limiter.New(client, "test:"+adapter.Name()+":"), AllowHTTP: true, AllowPrivate: true,
		Spacing: time.Millisecond,
	})
	return &harness{t, store, pool, client, server, adapter, fetcher}
}

func (h *harness) worker(name string) *Worker {
	return &Worker{Store: h.store, Fetch: h.fetcher, Valkey: h.valkey, Adapter: h.adapter, Consumer: name, Idle: 50 * time.Millisecond}
}

func (h *harness) board(token string) {
	h.t.Helper()
	if _, err := h.store.AddBoard(context.Background(), h.adapter.Name(), token); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) makeDue() {
	h.t.Helper()
	if _, err := h.pool.Exec(context.Background(), `UPDATE boards SET next_due_at = now() - interval '1 second', enqueued_at = NULL WHERE status = 'active'`); err != nil {
		h.t.Fatal(err)
	}
}

// cycle queues everything due and drains the stream with one worker.
func (h *harness) cycle() {
	h.t.Helper()
	ctx := context.Background()
	n, err := Enqueue(ctx, h.store, h.valkey, 100)
	if err != nil {
		h.t.Fatal(err)
	}
	w := h.worker("w1")
	for range n {
		if err := w.Step(ctx); err != nil {
			h.t.Fatal(err)
		}
	}
}

func (h *harness) count(where string) int {
	h.t.Helper()
	var n int
	if err := h.pool.QueryRow(context.Background(), `SELECT count(*) FROM postings WHERE `+where).Scan(&n); err != nil {
		h.t.Fatal(err)
	}
	return n
}

func TestIngestStoresNormalisedPostings(t *testing.T) {
	h := newHarness(t)
	h.server.set("acme", "1,2,3")
	h.board("acme")
	h.cycle()
	if got := h.count("closed_at IS NULL"); got != 3 {
		t.Fatalf("%d postings", got)
	}
	if got := h.count(`is_engineering AND 'IN' = ANY(countries) AND body = 'Build things'`); got != 3 {
		t.Fatalf("%d normalised postings", got)
	}
}

func TestSecondCycleUses304AndAddsNothing(t *testing.T) {
	h := newHarness(t)
	h.server.set("acme", "1,2,3")
	h.board("acme")
	h.cycle()
	h.makeDue()
	h.cycle()
	if got := h.count("true"); got != 3 {
		t.Fatalf("%d postings after a repeat", got)
	}
	var notModified int
	if err := h.pool.QueryRow(context.Background(), `SELECT count(*) FROM fetch_log WHERE not_modified AND bytes = 0`).Scan(&notModified); err != nil {
		t.Fatal(err)
	}
	if notModified != 1 {
		t.Fatalf("%d not-modified rows, want 1", notModified)
	}
}

func TestRemovedJobClosesAndAReturningJobReopens(t *testing.T) {
	h := newHarness(t)
	h.server.set("acme", "1,2,3")
	h.board("acme")
	h.cycle()
	h.server.set("acme", "1,2")
	h.makeDue()
	h.cycle()
	if got := h.count("closed_at IS NOT NULL"); got != 1 {
		t.Fatalf("%d closed", got)
	}
	h.server.set("acme", "1,2,3")
	h.makeDue()
	h.cycle()
	if got := h.count("closed_at IS NOT NULL"); got != 0 {
		t.Fatalf("%d still closed", got)
	}
}

func TestSuddenMassDropIsHeldOnceBeforeClosing(t *testing.T) {
	h := newHarness(t)
	h.server.set("acme", "1,2,3,4,5,6,7,8,9,10,11,12")
	h.board("acme")
	h.cycle()
	h.server.set("acme", "1")
	h.makeDue()
	h.cycle()
	if got := h.count("closed_at IS NOT NULL"); got != 0 {
		t.Fatalf("a first mass drop closed %d jobs", got)
	}
	h.makeDue()
	h.cycle()
	if got := h.count("closed_at IS NOT NULL"); got != 11 {
		t.Fatalf("a confirmed drop closed %d jobs, want 11", got)
	}
}

func TestMalformedResponseClosesNothing(t *testing.T) {
	h := newHarness(t)
	h.server.set("acme", "1,2,3")
	h.board("acme")
	h.cycle()
	h.server.bad["acme"] = true
	h.makeDue()
	h.cycle()
	if got := h.count("closed_at IS NULL"); got != 3 {
		t.Fatalf("%d active after a bad response", got)
	}
}

func TestBoardRetiresAfterThree404s(t *testing.T) {
	h := newHarness(t)
	h.server.set("gone", "1,2")
	h.board("gone")
	h.cycle()
	h.server.status["gone"] = http.StatusNotFound
	for range 3 {
		h.makeDue()
		h.cycle()
	}
	var status string
	if err := h.pool.QueryRow(context.Background(), `SELECT status FROM boards`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "retired" || h.count("closed_at IS NOT NULL") != 2 {
		t.Fatalf("status %s, %d closed", status, h.count("closed_at IS NOT NULL"))
	}
}

// Three failures from a dead connection must not look like three 404s.
func TestNetworkFailuresNeverRetireABoard(t *testing.T) {
	h := newHarness(t)
	h.server.set("acme", "1,2")
	h.board("acme")
	h.cycle()
	ctx := context.Background()
	var id int64
	if err := h.pool.QueryRow(ctx, `SELECT id FROM boards`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	for _, status := range []int{0, 0, 0, 0, 500, 502, 500} {
		attempt, err := h.store.Acquire(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if err := h.store.Fail(ctx, id, attempt, status); err != nil {
			t.Fatal(err)
		}
	}
	var state string
	var wait time.Duration
	if err := h.pool.QueryRow(ctx, `SELECT status, next_due_at - now() FROM boards`).Scan(&state, &wait); err != nil {
		t.Fatal(err)
	}
	if state != "active" || h.count("closed_at IS NOT NULL") != 0 {
		t.Fatalf("status %s, %d closed", state, h.count("closed_at IS NOT NULL"))
	}
	if wait > maxBackoff+time.Minute {
		t.Fatalf("backed off %v, over the %v cap", wait, maxBackoff)
	}
}

func TestStaleCursorRestartsTheWalk(t *testing.T) {
	h := newHarness(t)
	h.server.set("big", "1,2", "3,4", "5,6")
	h.board("big")
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	if _, err := Enqueue(ctx, h.store, h.valkey, 10); err != nil {
		t.Fatal(err)
	}
	w := h.worker("w")
	w.OnPage = func() { stop() }
	_ = w.Step(ctx)
	bg := context.Background()
	if _, err := h.pool.Exec(bg, `UPDATE boards SET next_url = $1`, h.server.server.URL+"/expired?page=9"); err != nil {
		t.Fatal(err)
	}
	h.server.status["expired"] = http.StatusBadRequest
	time.Sleep(80 * time.Millisecond)
	if err := h.worker("second").Step(bg); err != nil {
		t.Fatal(err)
	}
	var next *string
	if err := h.pool.QueryRow(bg, `SELECT next_url FROM boards`).Scan(&next); err != nil {
		t.Fatal(err)
	}
	if next != nil {
		t.Fatalf("a dead cursor must be dropped, still %q", *next)
	}
	if got := h.count("closed_at IS NOT NULL"); got != 0 {
		t.Fatalf("%d closed by a dead cursor", got)
	}
	h.makeDue()
	h.cycle()
	if got := h.count("closed_at IS NULL"); got != 6 {
		t.Fatalf("%d active after the restart", got)
	}
}

func TestDailySourceRefreshesDaily(t *testing.T) {
	if refreshFor("himalayas") != 24*time.Hour || refreshFor("lever") != RefreshEvery {
		t.Fatal("wrong refresh interval")
	}
}

func TestDenylistedBoardIsNeverFetched(t *testing.T) {
	h := newHarness(t)
	h.server.set("acme", "1")
	h.board("acme")
	if _, err := h.pool.Exec(context.Background(), `UPDATE boards SET denylisted = true`); err != nil {
		t.Fatal(err)
	}
	h.cycle()
	if len(h.server.requests()) != 0 {
		t.Fatal("a denylisted board was fetched")
	}
}

// Stops mid-run at every page in turn, the way a closed laptop would, then restarts.
func TestStopAtEveryPageResumesWithoutLossOrDuplicates(t *testing.T) {
	for stopAfter := 1; stopAfter <= 3; stopAfter++ {
		t.Run(fmt.Sprintf("after page %d", stopAfter), func(t *testing.T) {
			h := newHarness(t)
			h.server.set("big", "1,2", "3,4", "5,6", "7,8")
			h.board("big")
			ctx, stop := context.WithCancel(context.Background())
			defer stop()
			if _, err := Enqueue(ctx, h.store, h.valkey, 10); err != nil {
				t.Fatal(err)
			}
			first := h.worker("first")
			pages := 0
			first.OnPage = func() {
				pages++
				if pages == stopAfter {
					stop()
				}
			}
			_ = first.Step(ctx)

			if got := h.count("true"); got != stopAfter*2 {
				t.Fatalf("%d postings saved before the stop", got)
			}
			before := len(h.server.requests())

			time.Sleep(80 * time.Millisecond)
			second := h.worker("second")
			if err := second.Step(context.Background()); err != nil {
				t.Fatal(err)
			}
			if got := h.count("closed_at IS NULL"); got != 8 {
				t.Fatalf("%d active after resuming, want 8", got)
			}
			resumed := h.server.requests()[before:]
			if len(resumed) != 4-stopAfter {
				t.Fatalf("resume made %d requests, want %d: %v", len(resumed), 4-stopAfter, resumed)
			}
			if want := fmt.Sprintf("big?page=%d", stopAfter+1); resumed[0] != want {
				t.Fatalf("resumed at %s, want %s", resumed[0], want)
			}
		})
	}
}

// A saved position survives a stop even if the whole queue is lost with it.
func TestResumesAfterLosingTheQueue(t *testing.T) {
	h := newHarness(t)
	h.server.set("big", "1,2", "3,4", "5,6")
	h.board("big")
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	if _, err := Enqueue(ctx, h.store, h.valkey, 10); err != nil {
		t.Fatal(err)
	}
	w := h.worker("w")
	w.OnPage = func() { stop() }
	_ = w.Step(ctx)

	bg := context.Background()
	if err := h.valkey.Do(bg, h.valkey.B().Del().Key(Stream(h.adapter.Name())).Build()).Error(); err != nil {
		t.Fatal(err)
	}
	if err := Setup(bg, h.valkey, h.adapter.Name()); err != nil {
		t.Fatal(err)
	}
	if _, err := h.pool.Exec(bg, `UPDATE boards SET enqueued_at = now() - interval '1 hour'`); err != nil {
		t.Fatal(err)
	}
	before := len(h.server.requests())
	h.cycle()
	if got := h.count("closed_at IS NULL"); got != 6 {
		t.Fatalf("%d active", got)
	}
	if resumed := h.server.requests()[before:]; resumed[0] != "big?page=2" {
		t.Fatalf("restarted from %s", resumed[0])
	}
}

// A crash after the commit but before the ack redelivers a finished board.
func TestRedeliveryAfterCommitDoesNoWork(t *testing.T) {
	h := newHarness(t)
	h.server.set("acme", "1,2")
	h.board("acme")
	ctx := context.Background()
	if _, err := Enqueue(ctx, h.store, h.valkey, 10); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := h.pool.QueryRow(ctx, `SELECT id FROM boards`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	w := h.worker("w")
	if err := w.process(ctx, id); err != nil { // committed, never acknowledged
		t.Fatal(err)
	}
	before := len(h.server.requests())
	if err := w.Step(ctx); err != nil {
		t.Fatal(err)
	}
	if got := len(h.server.requests()); got != before {
		t.Fatalf("a finished board was fetched again: %d requests", got-before)
	}
}

// A worker that's overtaken mid-run must not close what it never read. This is the case
// the many-workers test first exposed.
func TestSupersededWorkerClosesNothing(t *testing.T) {
	h := newHarness(t)
	h.server.set("big", "1,2", "3,4", "5,6")
	h.board("big")
	ctx := context.Background()
	if _, err := Enqueue(ctx, h.store, h.valkey, 10); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := h.pool.QueryRow(ctx, `SELECT id FROM boards`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	slow, fast := h.worker("slow"), h.worker("fast")
	took := false
	slow.OnPage = func() {
		if !took {
			took = true
			if err := fast.process(ctx, id); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := slow.process(ctx, id); !errors.Is(err, ErrStale) {
		t.Fatalf("want ErrStale, got %v", err)
	}
	if got := h.count("closed_at IS NOT NULL"); got != 0 {
		t.Fatalf("a superseded worker closed %d live jobs", got)
	}
	if got := h.count("closed_at IS NULL"); got != 6 {
		t.Fatalf("%d active", got)
	}
}

func TestStaleWorkerCannotRecordFailure(t *testing.T) {
	h := newHarness(t)
	h.server.set("acme", "1")
	h.board("acme")
	ctx := context.Background()
	var id int64
	if err := h.pool.QueryRow(ctx, `SELECT id FROM boards`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	old, err := h.store.Acquire(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.Acquire(ctx, id); err != nil {
		t.Fatal(err)
	}
	for name, stale := range map[string]error{
		"Fail":        h.store.Fail(ctx, id, old, 500),
		"Defer":       h.store.Defer(ctx, id, old, time.Hour, 429),
		"NotModified": h.store.NotModified(ctx, id, old, "x"),
	} {
		if !errors.Is(stale, ErrStale) {
			t.Errorf("%s: want ErrStale, got %v", name, stale)
		}
	}
}

func (h *harness) posting(sourceName, board, id, company, title, location, body string) int64 {
	h.t.Helper()
	ctx := context.Background()
	var sourceID int16
	err := h.pool.QueryRow(ctx, `INSERT INTO sources (name) VALUES ($1)
		ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name RETURNING id`, sourceName).Scan(&sourceID)
	if err != nil {
		h.t.Fatal(err)
	}
	var postingID int64
	err = h.pool.QueryRow(ctx, `
		INSERT INTO postings (source_id, source_job_id, title, company, location_raw, is_engineering, body, content_hash, url)
		VALUES ($1, $2, $3, $4, $5, true, $6, '\\x00', 'https://x.example/' || $2) RETURNING id`,
		sourceID, board+":"+id, title, company, location, body).Scan(&postingID)
	if err != nil {
		h.t.Fatal(err)
	}
	return postingID
}

func longText(seed string) string {
	var b strings.Builder
	for i := range 80 {
		fmt.Fprintf(&b, "%s%d ", seed, i)
	}
	return b.String()
}

func TestDedupeGroupsTheSameJobOnly(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	text := longText("w")
	aggregator := h.posting("himalayas", "feed", "1", "Acme Inc", "Backend Engineer", "India", text)
	ats := h.posting("greenhouse", "acme", "1", "Acme", "Backend Engineer", "Pune", text)
	otherCity := h.posting("greenhouse", "acme", "2", "Acme", "Backend Engineer", "London", text)
	otherText := h.posting("greenhouse", "acme", "3", "Acme", "Backend Engineer", "Pune", longText("x"))
	short1 := h.posting("greenhouse", "acme", "4", "Acme", "Intern", "Pune", "Apply now")
	short2 := h.posting("greenhouse", "acme", "5", "Acme", "Intern", "Pune", "Apply now")

	done, merged, err := h.store.Dedupe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if done != 6 || merged != 1 {
		t.Fatalf("placed %d, merged %d", done, merged)
	}
	cluster := func(id int64) int64 {
		var c int64
		if err := h.pool.QueryRow(ctx, `SELECT cluster_id FROM postings WHERE id = $1`, id).Scan(&c); err != nil {
			t.Fatal(err)
		}
		return c
	}
	if cluster(aggregator) != cluster(ats) {
		t.Fatal("an aggregator's copy must join the ATS posting")
	}
	for name, id := range map[string]int64{"other city": otherCity, "other text": otherText} {
		if cluster(id) == cluster(ats) {
			t.Fatalf("%s must not merge", name)
		}
	}
	if cluster(short1) == cluster(short2) {
		t.Fatal("text too short to compare must not merge")
	}
	var canonical int64
	if err := h.pool.QueryRow(ctx, `SELECT canonical_posting_id FROM clusters WHERE id = $1`, cluster(ats)).Scan(&canonical); err != nil {
		t.Fatal(err)
	}
	if canonical != ats {
		t.Fatal("the employer's ATS posting must be canonical")
	}

	again, _, err := h.store.Dedupe(ctx)
	if err != nil || again != 0 {
		t.Fatalf("a rerun redid %d postings, err %v", again, err)
	}
}

func TestChangedTextLeavesItsCluster(t *testing.T) {
	h := newHarness(t)
	h.server.set("acme", "1")
	h.board("acme")
	h.cycle()
	ctx := context.Background()
	if _, _, err := h.store.Dedupe(ctx); err != nil {
		t.Fatal(err)
	}
	if got := h.count("cluster_id IS NOT NULL"); got != 1 {
		t.Fatalf("%d clustered", got)
	}
	if _, err := h.pool.Exec(ctx, `UPDATE postings SET content_hash = '\\x01'`); err != nil {
		t.Fatal(err)
	}
	h.server.set("acme", "1", "")
	h.makeDue()
	h.cycle()
	if got := h.count("cluster_id IS NULL"); got != 1 {
		t.Fatalf("changed text must leave its cluster, %d unclustered", got)
	}
}

func TestManyWorkersManyBoardsNoDuplicates(t *testing.T) {
	h := newHarness(t)
	const boards = 12
	for i := range boards {
		token := fmt.Sprintf("b%d", i)
		h.server.set(token, "1,2,3", "4,5")
		h.board(token)
	}
	ctx, stop := context.WithTimeout(context.Background(), 20*time.Second)
	defer stop()
	if _, err := Enqueue(ctx, h.store, h.valkey, boards); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := range 4 {
		w := h.worker(fmt.Sprintf("w%d", i))
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				_ = w.Step(ctx)
				if h.count("true") == boards*5 {
					return
				}
			}
		}()
	}
	wg.Wait()
	if got := h.count("closed_at IS NULL"); got != boards*5 {
		t.Fatalf("%d postings, want %d", got, boards*5)
	}
	var distinct int
	if err := h.pool.QueryRow(ctx, `SELECT count(DISTINCT source_job_id) FROM postings`).Scan(&distinct); err != nil {
		t.Fatal(err)
	}
	if distinct != boards*5 {
		t.Fatalf("%d distinct IDs", distinct)
	}
}
