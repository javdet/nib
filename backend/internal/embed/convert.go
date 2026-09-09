package embed

import (
	"errors"
)

// FloatsToFloat32 narrows a provider's float64 embedding row to the float32 form
// pgvector stores, returning the vector and its dimension. An empty row is an
// error: a zero-width vector would be accepted by the collection check and then
// fail row by row at insert.
func FloatsToFloat32(xs []float64) ([]float32, int, error) {
	if len(xs) == 0 {
		return nil, 0, errors.New("empty embedding vector")
	}
	out := make([]float32, len(xs))
	for i, v := range xs {
		out[i] = float32(v)
	}
	return out, len(out), nil
}

func truncateForErr(b []byte) string {
	const max = 512
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "…"
}
