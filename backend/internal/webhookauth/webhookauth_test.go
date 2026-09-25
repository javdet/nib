package webhookauth

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestVerify(t *testing.T) {
	t.Parallel()

	key := []byte("backend-only-key")
	const chat, job = "3f1c7a52-6f5e-4d2a-9c1b-0a4b5c6d7e8f", "nib-12345678"
	token := Sign(key, chat, job)

	tests := []struct {
		name  string
		key   []byte
		chat  string
		job   string
		token string
		want  bool
	}{
		{name: "own run", key: key, chat: chat, job: job, token: token, want: true},
		{name: "another chat", key: key, chat: "0e2d3c4b-5a69-4788-9aab-bccddeeff001", job: job, token: token},
		{name: "another job", key: key, chat: chat, job: "nib-87654321", token: token},
		{name: "another key", key: []byte("other"), chat: chat, job: job, token: token},
		{name: "raw key as token", key: key, chat: chat, job: job, token: string(key)},
		{name: "empty token", key: key, chat: chat, job: job},
		{name: "empty job", key: key, chat: chat, token: Sign(key, chat, "")},
		{name: "empty chat", key: key, job: job, token: Sign(key, "", job)},
		{name: "empty key", chat: chat, job: job, token: Sign(nil, chat, job)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Verify(tt.key, tt.chat, tt.job, tt.token); got != tt.want {
				t.Fatalf("Verify = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadOrCreateKeyGeneratesOnce(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "data")
	first, created, err := LoadOrCreateKey(dir, "")
	if err != nil {
		t.Fatalf("first LoadOrCreateKey: %v", err)
	}
	if !created {
		t.Fatalf("created = false on an empty data dir, want true")
	}
	if len(first) != 64 {
		t.Fatalf("key length = %d, want 64 hex chars", len(first))
	}

	info, err := os.Stat(filepath.Join(dir, KeyFileName))
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("key file mode = %o, want 600", perm)
	}

	second, created, err := LoadOrCreateKey(dir, "")
	if err != nil {
		t.Fatalf("second LoadOrCreateKey: %v", err)
	}
	if created {
		t.Fatalf("created = true on reuse, want false")
	}
	if string(second) != string(first) {
		t.Fatalf("key changed between loads")
	}
}

func TestLoadOrCreateKeyOverrideWins(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	key, created, err := LoadOrCreateKey(dir, "  from-env  ")
	if err != nil {
		t.Fatalf("LoadOrCreateKey: %v", err)
	}
	if string(key) != "from-env" || created {
		t.Fatalf("key, created = %q, %v, want %q, false", key, created, "from-env")
	}
	if _, err := os.Stat(filepath.Join(dir, KeyFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("override wrote a key file (stat err %v)", err)
	}
}

func TestLoadOrCreateKeyRefusesEmptyFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, KeyFileName), []byte("\n"), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}
	if _, _, err := LoadOrCreateKey(dir, ""); !errors.Is(err, ErrEmptyKeyFile) {
		t.Fatalf("err = %v, want ErrEmptyKeyFile", err)
	}
}
