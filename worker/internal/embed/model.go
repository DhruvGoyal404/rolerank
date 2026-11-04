package embed

// The pinned model (ARCHITECTURE.md, "Embeddings"). Files come from
// https://huggingface.co/BAAI/bge-small-en-v1.5 at Revision; scripts/models.sh downloads them
// into models/ and checks each SHA-256 before use.
const (
	Model       = "BAAI/bge-small-en-v1.5"
	Revision    = "5c38ec7c405ec4b44b94cc5a9bb96e735b38267a"
	ModelSHA256 = "828e1496d7fabb79cfa4dcd84fa38625c0d3d21da474a00f08db0f559940cf35" // onnx/model.onnx
	VocabSHA256 = "07eced375cec144d27c900241f3e339478dec958f92fddbc551f295c992038a3"
	Dims        = 384
	MaxTokens   = 512
	// ModelTag is stored with every vector, so a model switch is a re-embed, not a migration.
	ModelTag = "bge-small-en-v1.5@5c38ec7"
)
