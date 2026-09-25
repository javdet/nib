package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/javdet/nib/internal/mcpconfig"
)

// A secret's allowed hosts decide where mcp.json may send it. Whoever can edit
// mcp.json can edit this list too, so adding a host has to take the value.
func TestSecretServiceUpdateAllowedHosts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		hosts     []string
		value     string
		wantErr   error
		wantHosts []string
	}{
		{"add without value", []string{"api.github.com", "evil.test"}, "", ErrSecretHostsNeedValue, nil},
		{"replace without value", []string{"evil.test"}, "", ErrSecretHostsNeedValue, nil},
		{"add with value", []string{"api.github.com", "uploads.github.com"}, "ghp_new", nil, []string{"api.github.com", "uploads.github.com"}},
		{"remove without value", []string{}, "", nil, []string{}},
		{"same list respelled without value", []string{"API.GitHub.com."}, "", nil, []string{"api.github.com"}},
		{"absent keeps stored list", nil, "", nil, []string{"api.github.com"}},
		{"invalid host", []string{"https://api.github.com"}, "ghp_new", mcpconfig.ErrInvalidAllowedHost, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := NewSecretService(newFakeSecretRepo(), testCipher(t))
			created, err := svc.Create(context.Background(), SecretInput{
				Name:         "GITHUB_TOKEN",
				Value:        "ghp_old",
				AllowedHosts: []string{"api.github.com"},
			})
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}

			updated, err := svc.Update(context.Background(), created.ID, SecretInput{
				Name:         "GITHUB_TOKEN",
				Value:        tt.value,
				AllowedHosts: tt.hosts,
			})
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Update() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Update() error = %v", err)
			}
			if !reflect.DeepEqual(updated.AllowedHosts, tt.wantHosts) {
				t.Fatalf("AllowedHosts = %#v, want %#v", updated.AllowedHosts, tt.wantHosts)
			}
			got, err := svc.AllowedHostsByName(context.Background(), "", "", "GITHUB_TOKEN")
			if err != nil {
				t.Fatalf("AllowedHostsByName() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.wantHosts) {
				t.Fatalf("AllowedHostsByName() = %#v, want %#v", got, tt.wantHosts)
			}
		})
	}
}

func TestSecretServiceCreateWithoutHostsBindsNone(t *testing.T) {
	t.Parallel()

	svc := NewSecretService(newFakeSecretRepo(), testCipher(t))
	if _, err := svc.Create(context.Background(), SecretInput{Name: "GIT_TOKEN", Value: "glpat"}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	got, err := svc.AllowedHostsByName(context.Background(), "", "", "GIT_TOKEN")
	if err != nil {
		t.Fatalf("AllowedHostsByName() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("AllowedHostsByName() = %#v, want none", got)
	}
}
