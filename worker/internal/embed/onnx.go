//go:build onnx

package embed

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	ort "github.com/yalue/onnxruntime_go"
)

// Embedder runs the pinned model through ONNX Runtime. It needs cgo and the runtime library,
// so it builds only with -tags onnx, inside the Linux container (scripts/embed.sh).
type Embedder struct {
	tok  *Tokenizer
	sess *ort.DynamicAdvancedSession
}

// Open checks the model file against ModelSHA256 and starts a session.
func Open(library, model string, threads int) (*Embedder, error) {
	if err := checkSHA(model, ModelSHA256); err != nil {
		return nil, err
	}
	ort.SetSharedLibraryPath(library)
	if err := ort.InitializeEnvironment(); err != nil {
		return nil, fmt.Errorf("onnx runtime: %w", err)
	}
	opts, err := ort.NewSessionOptions()
	if err != nil {
		return nil, err
	}
	defer opts.Destroy()
	if err := opts.SetIntraOpNumThreads(threads); err != nil {
		return nil, err
	}
	sess, err := ort.NewDynamicAdvancedSession(model,
		[]string{"input_ids", "attention_mask", "token_type_ids"}, []string{"last_hidden_state"}, opts)
	if err != nil {
		return nil, fmt.Errorf("onnx session: %w", err)
	}
	return &Embedder{tok: NewTokenizer(), sess: sess}, nil
}

// Embed returns one unit vector per text.
func (e *Embedder) Embed(texts []string) ([][]float32, error) {
	rows := make([][]int64, len(texts))
	for i, t := range texts {
		rows[i] = e.tok.Encode(t, MaxTokens)
	}
	ids, mask, types, seq := batchInputs(rows, e.tok.pad)
	shape := ort.NewShape(int64(len(texts)), int64(seq))
	var inputs []ort.Value
	for _, data := range [][]int64{ids, mask, types} {
		t, err := ort.NewTensor(shape, data)
		if err != nil {
			return nil, err
		}
		defer t.Destroy()
		inputs = append(inputs, t)
	}
	out, err := ort.NewEmptyTensor[float32](ort.NewShape(int64(len(texts)), int64(seq), Dims))
	if err != nil {
		return nil, err
	}
	defer out.Destroy()
	if err := e.sess.Run(inputs, []ort.Value{out}); err != nil {
		return nil, fmt.Errorf("onnx run: %w", err)
	}
	return pool(out.GetData(), len(texts), seq, Dims), nil
}

// Close ends the session and the runtime.
func (e *Embedder) Close() error {
	e.sess.Destroy()
	return ort.DestroyEnvironment()
}

func checkSHA(path, want string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("%s has SHA-256 %s, want %s", path, got, want)
	}
	return nil
}
