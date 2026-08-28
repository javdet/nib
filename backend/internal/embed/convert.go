package embed

import (
	"errors"
)

func floatsToFloat32(xs []float64) ([]float32, int, error) {
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
