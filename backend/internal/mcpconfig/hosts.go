package mcpconfig

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"
)

// ErrSecretHostNotAllowed is returned when a ${NAME} reference would send a
// secret to a host the secret is not bound to. mcp.json is editable by anyone
// holding the API token while secret values are write-only, so without this an
// edit pointing a server at another host is a way to read any secret back out.
var ErrSecretHostNotAllowed = errors.New("secret is not allowed for this host")

// ErrSecretInURLHost is returned for a ${NAME} reference in a server URL's
// scheme, host or port. The host is what a secret is bound to, so it has to be
// literal: a host spelled from a secret could be anything.
var ErrSecretInURLHost = errors.New("variable reference in server URL host")

// ErrInvalidAllowedHost marks an allowed-hosts entry that is not a bare hostname.
var ErrInvalidAllowedHost = errors.New("invalid allowed host")

// NormalizeAllowedHosts validates a secret's allowed hosts and returns them
// lowercased, deduplicated and sorted. Entries are bare hostnames or IP
// addresses — no scheme, port, path or wildcard — compared exactly against the
// host of a server URL. A nil input stays nil so callers can tell "not
// supplied" from "none".
func NormalizeAllowedHosts(hosts []string) ([]string, error) {
	if hosts == nil {
		return nil, nil
	}
	out := make([]string, 0, len(hosts))
	for _, raw := range hosts {
		h := strings.TrimSpace(raw)
		if h == "" {
			continue
		}
		norm, ok := normalizeHost(h)
		if !ok {
			return nil, fmt.Errorf(
				"%w: %q — enter a hostname such as api.example.com, without scheme, port or path",
				ErrInvalidAllowedHost, h,
			)
		}
		out = append(out, norm)
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

func normalizeHost(h string) (string, bool) {
	h = strings.TrimSuffix(strings.ToLower(h), ".")
	if strings.HasPrefix(h, "[") && strings.HasSuffix(h, "]") {
		h = h[1 : len(h)-1]
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.String(), true
	}
	if h == "" || len(h) > 253 {
		return "", false
	}
	for _, label := range strings.Split(h, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", false
		}
		for i := 0; i < len(label); i++ {
			b := label[i]
			if !(b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '-' || b == '_') {
				return "", false
			}
		}
	}
	return h, true
}

// HostAllowed reports whether host is on the list. host is expected in the
// form serverHost returns.
func HostAllowed(allowed []string, host string) bool {
	return host != "" && slices.Contains(allowed, host)
}

// serverHost returns the literal host of a raw, unexpanded server URL, and
// refuses a URL whose scheme, host or port carries a ${NAME} reference. The
// userinfo and everything after the authority may still carry references:
// they are sent to the host, not used to pick it.
func serverHost(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", nil
	}
	schemeEnd := strings.Index(rawURL, "://")
	if schemeEnd < 0 {
		if strings.Contains(rawURL, "${") {
			return "", fmt.Errorf("%w: %q has no scheme and host to bind its secrets to", ErrSecretInURLHost, rawURL)
		}
		return "", nil
	}
	authority := rawURL[schemeEnd+3:]
	if i := strings.IndexAny(authority, "/?#"); i >= 0 {
		authority = authority[:i]
	}
	hostPort := authority
	if i := strings.LastIndexByte(authority, '@'); i >= 0 {
		hostPort = authority[i+1:]
	}
	if strings.Contains(rawURL[:schemeEnd], "${") || strings.Contains(hostPort, "${") {
		return "", fmt.Errorf(
			"%w: write the scheme and host literally and keep ${NAME} to the path, query, userinfo or headers",
			ErrSecretInURLHost,
		)
	}
	u, err := url.Parse(rawURL[:schemeEnd+3] + hostPort)
	if err != nil {
		return "", nil
	}
	host, ok := normalizeHost(u.Hostname())
	if !ok {
		return "", nil
	}
	return host, nil
}

// SecretHostRefs maps each secret name referenced by a server to the hosts of
// the servers referencing it, in the form HostAllowed compares. It is what an
// upgrade binds existing secrets from, so that the mcp.json already in place
// keeps working. Servers with no literal host are skipped.
func SecretHostRefs(servers []Server) map[string][]string {
	refs := map[string][]string{}
	for _, srv := range servers {
		host, err := serverHost(srv.URL)
		if err != nil || host == "" {
			continue
		}
		names := referencedNames(srv.URL)
		for _, v := range srv.Headers {
			names = append(names, referencedNames(v)...)
		}
		for _, name := range names {
			if !slices.Contains(refs[name], host) {
				refs[name] = append(refs[name], host)
			}
		}
	}
	for name := range refs {
		slices.Sort(refs[name])
	}
	return refs
}

// referencedNames lists the ${NAME} references expandString would look up.
func referencedNames(s string) []string {
	var names []string
	for i := 0; i < len(s); {
		if strings.HasPrefix(s[i:], "$${") {
			i += 3
			continue
		}
		if !strings.HasPrefix(s[i:], "${") {
			i++
			continue
		}
		end := strings.IndexByte(s[i+2:], '}')
		if end < 0 {
			break
		}
		if name := s[i+2 : i+2+end]; varRef.MatchString(name) {
			names = append(names, name)
		}
		i += 2 + end + 1
	}
	return names
}
