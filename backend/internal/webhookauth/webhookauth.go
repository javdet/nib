// Package webhookauth issues and checks the per-run tokens agent-runner
// containers authenticate their completion webhook with.
//
// A container is handed a token derived from one backend-only key and bound to
// its own chat id and job name, never the key itself. The agent inside can read
// its environment, so a shared token would let any run forge the result of any
// other; a derived one only ever vouches for the run it was issued to.
package webhookauth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/javdet/nib/internal/atomicfile"
)

// KeyFileName is where a generated key is kept, under DATA_DIR.
const KeyFileName = ".webhook-key"

const signingDomain = "nib-agent-webhook"

// ErrEmptyKeyFile is returned for a key file that exists but holds nothing.
var ErrEmptyKeyFile = errors.New("webhook key file is empty")

// LoadOrCreateKey returns the signing key. A non-empty override
// (AGENT_WEBHOOK_TOKEN) wins; otherwise the key is read from
// {dataDir}/.webhook-key and generated there on first use. created reports that
// the file was just written.
//
// A file that exists but is empty or unreadable is an error rather than a
// reason to generate a new key: replacing it would silently orphan every run
// still in flight.
func LoadOrCreateKey(dataDir, override string) (key []byte, created bool, err error) {
	if v := strings.TrimSpace(override); v != "" {
		return []byte(v), false, nil
	}

	path := filepath.Join(dataDir, KeyFileName)
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		v := strings.TrimSpace(string(data))
		if v == "" {
			return nil, false, fmt.Errorf("read %s: %w", path, ErrEmptyKeyFile)
		}
		return []byte(v), false, nil
	case !errors.Is(err, os.ErrNotExist):
		return nil, false, fmt.Errorf("read %s: %w", path, err)
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, false, fmt.Errorf("generate webhook key: %w", err)
	}
	v := hex.EncodeToString(raw)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, false, fmt.Errorf("create %s: %w", dataDir, err)
	}
	// atomicfile writes through os.CreateTemp, so the file lands 0600.
	if err := atomicfile.WriteString(path, v+"\n"); err != nil {
		return nil, false, fmt.Errorf("write %s: %w", path, err)
	}
	return []byte(v), true, nil
}

// Sign returns the token for one run.
func Sign(key []byte, chatID, jobName string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(signingDomain + "\x00" + chatID + "\x00" + jobName))
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify reports whether token was issued for this chat id and job name. An
// empty key, id, name or token never verifies.
func Verify(key []byte, chatID, jobName, token string) bool {
	if len(key) == 0 || chatID == "" || jobName == "" || token == "" {
		return false
	}
	return hmac.Equal([]byte(token), []byte(Sign(key, chatID, jobName)))
}
