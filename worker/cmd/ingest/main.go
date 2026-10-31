// Command ingest will run the ingestion workers. P2 fills it in.
package main

import (
	"fmt"

	"github.com/DhruvGoyal404/rolerank/worker/internal/useragent"
)

var version = "dev"

func main() {
	fmt.Println(useragent.String(version))
}
