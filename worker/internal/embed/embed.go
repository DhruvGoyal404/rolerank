package embed

import (
	"math"
	"strconv"
	"strings"
)

// PostingText is what a job is embedded from: title, required skills, then the text. The
// tokenizer keeps the first MaxTokens tokens, so the title and skills always fit.
func PostingText(title string, skills []string, body string) string {
	return title + "\n" + strings.Join(skills, ", ") + "\n" + body
}

// QueryPrefix is bge-v1.5's instruction for short retrieval queries. Whether profiles use it is
// decided by the P3 bake-off on D3, not assumed.
const QueryPrefix = "Represent this sentence for searching relevant passages: "

// pool takes the [CLS] vector of each row of a [batch, seq, dims] output and scales it to unit
// length, which is how bge-small-en-v1.5 is meant to be used.
func pool(hidden []float32, batch, seq, dims int) [][]float32 {
	out := make([][]float32, batch)
	for b := range batch {
		v := make([]float32, dims)
		copy(v, hidden[b*seq*dims:b*seq*dims+dims])
		var sum float64
		for _, x := range v {
			sum += float64(x) * float64(x)
		}
		if norm := math.Sqrt(sum); norm > 0 {
			for i := range v {
				v[i] = float32(float64(v[i]) / norm)
			}
		}
		out[b] = v
	}
	return out
}

// Literal formats a vector for a halfvec or vector parameter.
func Literal(v []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(x), 'g', 6, 32))
	}
	b.WriteByte(']')
	return b.String()
}

// batchInputs pads token rows to the longest, returning ids, attention mask and token types.
func batchInputs(rows [][]int64, pad int64) (ids, mask, types []int64, seq int) {
	for _, r := range rows {
		seq = max(seq, len(r))
	}
	ids = make([]int64, 0, len(rows)*seq)
	mask = make([]int64, 0, len(rows)*seq)
	types = make([]int64, len(rows)*seq)
	for _, r := range rows {
		for i := range seq {
			if i < len(r) {
				ids, mask = append(ids, r[i]), append(mask, 1)
			} else {
				ids, mask = append(ids, pad), append(mask, 0)
			}
		}
	}
	return ids, mask, types, seq
}
