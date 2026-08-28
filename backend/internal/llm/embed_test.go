package llm

import "testing"

func TestFloatsToFloat32(t *testing.T) {
	t.Parallel()

	got, dim, err := floatsToFloat32([]float64{0.5, -1.25, 2})
	if err != nil {
		t.Fatalf("floatsToFloat32() error = %v", err)
	}
	if dim != 3 {
		t.Errorf("dimension = %d, want 3", dim)
	}
	want := []float32{0.5, -1.25, 2}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %v, want %v", i, got[i], want[i])
		}
	}

	if _, _, err := floatsToFloat32(nil); err == nil {
		t.Fatal("floatsToFloat32(nil) expected error")
	}
}
