//go:build !onnx

package main

import (
	"fmt"
	"os"
)

// Without ONNX Runtime there is no embedder: build with -tags onnx (compose service "ranker").
func main() {
	fmt.Fprintln(os.Stderr, "ranker: built without -tags onnx; run it in the ranker container")
	os.Exit(2)
}
