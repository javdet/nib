package embed

import "testing"

func TestFloatsToFloat32(t *testing.T) {
	t.Parallel()

	got, dim, err := FloatsToFloat32([]float64{0.5, -1.25, 2})
	if err != nil {
		t.Fatalf("FloatsToFloat32() error = %v", err)
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

	if _, _, err := FloatsToFloat32(nil); err == nil {
		t.Fatal("FloatsToFloat32(nil) expected error")
	}
}
