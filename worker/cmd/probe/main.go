// Command probe reads one page of a source through the real fetcher, then repeats the
// request with the returned ETag. It prints counts only, never job text.
//
//	ROLERANK_VALKEY_URL=... probe <source> <token>
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/valkey-io/valkey-go"

	"github.com/DhruvGoyal404/rolerank/worker/internal/fetch"
	"github.com/DhruvGoyal404/rolerank/worker/internal/limiter"
	"github.com/DhruvGoyal404/rolerank/worker/internal/source"
	"github.com/DhruvGoyal404/rolerank/worker/internal/useragent"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: probe <source> <token>")
	}
	var adapter source.Adapter
	var hosts []string
	for _, a := range source.All() {
		hosts = append(hosts, a.Hosts()...)
		if a.Name() == args[0] {
			adapter = a
		}
	}
	if adapter == nil {
		return fmt.Errorf("unknown source %q", args[0])
	}
	options, err := valkey.ParseURL(os.Getenv("ROLERANK_VALKEY_URL"))
	if err != nil {
		return err
	}
	client, err := valkey.NewClient(options)
	if err != nil {
		return err
	}
	defer client.Close()

	fetcher := fetch.New(fetch.Options{
		UserAgent: useragent.String("0.1.0"),
		Agent:     "RoleRankBot",
		Hosts:     hosts,
		Limiter:   limiter.New(client, "rolerank:"),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	target := adapter.FirstURL(args[1])
	result, err := fetcher.Get(ctx, target, "")
	if err != nil {
		return err
	}
	page, err := adapter.Parse(args[1], result.Body)
	if err != nil {
		return err
	}
	withBody, withEmployment, withDate := 0, 0, 0
	for _, p := range page.Postings {
		if p.Body != "" {
			withBody++
		}
		if p.EmploymentType != "" {
			withEmployment++
		}
		if !p.PostedAt.IsZero() {
			withDate++
		}
	}
	fmt.Printf("%s: status=%d bytes=%d postings=%d dropped=%d skipped=%d withBody=%d withType=%d withDate=%d next=%t etag=%t\n",
		adapter.Name(), result.Status, len(result.Body), len(page.Postings), page.Dropped, page.Skipped,
		withBody, withEmployment, withDate, page.NextURL != "", result.ETag != "")
	if result.ETag == "" {
		return nil
	}
	again, err := fetcher.Get(ctx, target, result.ETag)
	if err != nil {
		return err
	}
	fmt.Printf("%s: conditional status=%d notModified=%t bytes=%d\n", adapter.Name(), again.Status, again.NotModified, len(again.Body))
	return nil
}
