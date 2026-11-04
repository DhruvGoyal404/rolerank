// Command d2 prints D2's texts as {"id","text"} lines, for scripts/embed.sh text. Profiles get
// ids "P..", jobs "J..". The vectors are stored in internal/d2/testdata so CI needs no model.
package main

import (
	"encoding/json"
	"os"

	"github.com/DhruvGoyal404/rolerank/worker/internal/d2"
	"github.com/DhruvGoyal404/rolerank/worker/internal/embed"
)

func main() {
	ps, js := d2.Generate(d2.Seed, d2.Profiles, d2.Jobs)
	out := json.NewEncoder(os.Stdout)
	type item struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	}
	for _, p := range ps {
		_ = out.Encode(item{p.ID, p.Text})
	}
	for _, j := range js {
		_ = out.Encode(item{j.ID, embed.PostingText(j.Title, j.Required, j.Body)})
	}
}
