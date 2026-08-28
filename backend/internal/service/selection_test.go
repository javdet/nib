package service

import (
	"testing"

	"github.com/javdet/nib/internal/domain"
)

func TestSelectionStore_defaultsToAny(t *testing.T) {
	t.Parallel()
	st := NewSelectionStore()
	got := st.Get()
	want := domain.Selection{
		Project:     "any",
		Environment: "any",
		Cloud:       "any",
		Location:    "any",
	}
	if got != want {
		t.Fatalf("Get() = %+v, want %+v", got, want)
	}
}

func TestSelectionStore_setSpecificValues(t *testing.T) {
	t.Parallel()
	st := NewSelectionStore()
	got := st.Set(domain.Selection{
		Project:     "myproject2",
		Environment: "stage",
		Cloud:       "digitalocean",
		Location:    "fra1",
	})
	want := domain.Selection{
		Project:     "myproject2",
		Environment: "stage",
		Cloud:       "digitalocean",
		Location:    "fra1",
	}
	if got != want {
		t.Fatalf("Set() = %+v, want %+v", got, want)
	}
	if st.Get() != want {
		t.Fatalf("Get() after Set = %+v, want %+v", st.Get(), want)
	}
}

func TestSelectionStore_projectAnyCascades(t *testing.T) {
	t.Parallel()
	st := NewSelectionStore()
	got := st.Set(domain.Selection{
		Project:     "any",
		Environment: "stage",
		Cloud:       "digitalocean",
		Location:    "fra1",
	})
	want := domain.Selection{
		Project:     "any",
		Environment: "any",
		Cloud:       "any",
		Location:    "any",
	}
	if got != want {
		t.Fatalf("Set() = %+v, want %+v", got, want)
	}
}

func TestSelectionStore_cloudAnyCascadesLocation(t *testing.T) {
	t.Parallel()
	st := NewSelectionStore()
	got := st.Set(domain.Selection{
		Project:     "myproject2",
		Environment: "stage",
		Cloud:       "any",
		Location:    "fra1",
	})
	want := domain.Selection{
		Project:     "myproject2",
		Environment: "stage",
		Cloud:       "any",
		Location:    "any",
	}
	if got != want {
		t.Fatalf("Set() = %+v, want %+v", got, want)
	}
}

func TestSelectionStore_emptyFieldsBecomeAny(t *testing.T) {
	t.Parallel()
	st := NewSelectionStore()
	got := st.Set(domain.Selection{
		Project:     "myproject2",
		Environment: "",
		Cloud:       "  ",
		Location:    "fra1",
	})
	want := domain.Selection{
		Project:     "myproject2",
		Environment: "any",
		Cloud:       "any",
		Location:    "any",
	}
	if got != want {
		t.Fatalf("Set() = %+v, want %+v", got, want)
	}
}
