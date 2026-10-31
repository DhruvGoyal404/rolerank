// Command leverscan runs Lever discovery step 1: a curated list of Indian companies from
// Wikipedia, one try each on Lever, and every hit checked against the company's own site.
// Results go to <data dir>/lever-scan.tsv, and only verified boards to lever-verified.txt.
// It's safe to stop and rerun, because finished companies are skipped.
//
//	ROLERANK_VALKEY_URL=... leverscan <data dir>
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/valkey-io/valkey-go"

	"github.com/DhruvGoyal404/rolerank/worker/internal/fetch"
	"github.com/DhruvGoyal404/rolerank/worker/internal/leverscan"
	"github.com/DhruvGoyal404/rolerank/worker/internal/limiter"
	"github.com/DhruvGoyal404/rolerank/worker/internal/useragent"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "leverscan:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: leverscan <data dir>")
	}
	dir := args[0]
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	options, err := valkey.ParseURL(os.Getenv("ROLERANK_VALKEY_URL"))
	if err != nil {
		return fmt.Errorf("ROLERANK_VALKEY_URL is missing or invalid")
	}
	client, err := valkey.NewClient(options)
	if err != nil {
		return fmt.Errorf("connect to valkey failed")
	}
	defer client.Close()
	shared := limiter.New(client, "rolerank:")
	newFetcher := func(hosts ...string) *fetch.Client {
		return fetch.New(fetch.Options{
			UserAgent: useragent.String("0.1.0"), Agent: "RoleRankBot", Hosts: hosts,
			Limiter: shared, Accept: "text/html,application/json",
		})
	}
	wiki, lever := newFetcher("en.wikipedia.org"), newFetcher("api.lever.co")

	seen := map[string]bool{}
	var companies []leverscan.Company
	for _, category := range leverscan.Categories {
		res, err := wiki.Get(ctx, "https://en.wikipedia.org/wiki/"+category, "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", category, err)
			continue
		}
		if res.Status != 200 {
			fmt.Printf("%s: status %d, skipped\n", category, res.Status)
			continue
		}
		list := leverscan.CategoryPages(res.Body)
		fmt.Printf("%s: %d names\n", category, len(list))
		for _, c := range list {
			if token := leverscan.Token(c.Name); token != "" && !seen[token] {
				seen[token] = true
				companies = append(companies, c)
			}
		}
	}
	sort.Slice(companies, func(i, j int) bool { return companies[i].Name < companies[j].Name })
	fmt.Printf("%d distinct names\n", len(companies))

	path := filepath.Join(dir, "lever-scan.tsv")
	done := map[string]bool{}
	if file, err := os.Open(path); err == nil {
		lines := bufio.NewScanner(file)
		for lines.Scan() {
			done[strings.SplitN(lines.Text(), "\t", 2)[0]] = true
		}
		file.Close()
	}
	out, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()

	tally := map[string]int{}
	for _, c := range companies {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		token := leverscan.Token(c.Name)
		if done[token] {
			continue
		}
		hit, err := leverscan.Hit(ctx, lever, token)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", token, err)
			continue
		}
		outcome := leverscan.Outcome("no_board")
		if hit {
			if res, err := wiki.Get(ctx, c.Page, ""); err == nil && res.Status == 200 {
				c.Site = leverscan.OfficialSite(res.Body)
			}
			outcome = leverscan.Verify(ctx, newFetcher(leverscan.Hosts(c.Site)...), c.Site, token)
		}
		tally[string(outcome)]++
		if _, err := fmt.Fprintln(out, leverscan.Line(c, token, outcome)); err != nil {
			return err
		}
		if outcome == leverscan.Verified {
			fmt.Printf("verified %s (%s)\n", token, c.Name)
		}
	}
	fmt.Printf("done: %v\n", tally)

	lines, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var verified []string
	for _, line := range strings.Split(string(lines), "\n") {
		parts := strings.Split(line, "\t")
		if len(parts) > 1 && parts[1] == string(leverscan.Verified) {
			verified = append(verified, parts[0])
		}
	}
	return os.WriteFile(filepath.Join(dir, "lever-verified.txt"), []byte(strings.Join(verified, "\n")+"\n"), 0o600)
}
