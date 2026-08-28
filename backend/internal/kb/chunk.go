package kb

const (
	// DefaultChunkSize is the maximum rune count per chunk for document ingest.
	DefaultChunkSize = 512
	// DefaultChunkOverlap is the rune overlap between consecutive chunks.
	DefaultChunkOverlap = 64
)

// Chunk splits s into overlapping segments of at most size runes. Overlap is
// applied between consecutive chunks (in runes). Empty input yields a nil slice.
func Chunk(s string, size, overlap int) []string {
	if size < 1 {
		size = 1
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= size {
		overlap = size - 1
	}
	r := []rune(s)
	if len(r) == 0 {
		return nil
	}
	step := size - overlap
	if step < 1 {
		step = 1
	}
	var out []string
	for i := 0; i < len(r); i += step {
		end := i + size
		if end > len(r) {
			end = len(r)
		}
		out = append(out, string(r[i:end]))
		if end == len(r) {
			break
		}
	}
	return out
}
