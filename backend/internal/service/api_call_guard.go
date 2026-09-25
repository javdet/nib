package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// api_call hands its argv to curl, and curl has options that read any file (-d @f, -H @f, -K),
// write any file (-o, -D, --trace), talk to the Docker socket (--unix-socket) or reroute the
// connection (--resolve, --connect-to, -x). So nothing reaches curl unless it is on this list, and
// every value that curl would take as a file name is refused. An unknown option is refused rather
// than passed: a blocklist would silently open up with every new curl release.

// curlValueCheck vets the argument of an option. A nil check marks an option that takes none.
type curlValueCheck func(string) error

var errCurlValueFromFile = errors.New("reading a value from a file is not allowed")

var curlLongOptions = map[string]curlValueCheck{
	"--silent":            nil,
	"--show-error":        nil,
	"--include":           nil,
	"--head":              nil,
	"--fail":              nil,
	"--fail-with-body":    nil,
	"--insecure":          nil,
	"--verbose":           nil,
	"--get":               nil,
	"--no-buffer":         nil,
	"--compressed":        nil,
	"--no-progress-meter": nil,
	"--http1.1":           nil,
	"--http2":             nil,
	"--request":           checkCurlMethod,
	"--header":            checkCurlNotFromFile,
	"--data":              checkCurlNotFromFile,
	"--data-ascii":        checkCurlNotFromFile,
	"--data-binary":       checkCurlNotFromFile,
	"--data-raw":          checkCurlAny,
	"--data-urlencode":    checkCurlURLEncode,
	"--json":              checkCurlNotFromFile,
	"--user":              checkCurlAny,
	"--oauth2-bearer":     checkCurlAny,
	"--user-agent":        checkCurlAny,
	"--referer":           checkCurlAny,
	"--cookie":            checkCurlCookie,
	"--max-time":          checkCurlNumber,
	"--connect-timeout":   checkCurlNumber,
	"--retry":             checkCurlNumber,
	"--write-out":         checkCurlWriteOut,
	"--output":            checkCurlOutput,
	"--url":               checkCurlAny,
}

var curlShortOptions = map[byte]string{
	's': "--silent",
	'S': "--show-error",
	'i': "--include",
	'I': "--head",
	'f': "--fail",
	'k': "--insecure",
	'v': "--verbose",
	'G': "--get",
	'N': "--no-buffer",
	'X': "--request",
	'H': "--header",
	'd': "--data",
	'u': "--user",
	'A': "--user-agent",
	'e': "--referer",
	'b': "--cookie",
	'm': "--max-time",
	'w': "--write-out",
	'o': "--output",
}

// curlRedirectOptions get their own refusal: a redirect is a second request to a host nobody
// vetted, so the caller has to make it themselves.
var curlRedirectOptions = map[string]bool{"-L": true, "--location": true, "--location-trusted": true}

// apiCallBlockedPrefixes adds to the loopback and link-local ranges (which already hold
// 169.254.169.254, the metadata address of AWS, GCP, Azure and most others) the metadata
// endpoints that live elsewhere.
var apiCallBlockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.100.100.200/32"), // Alibaba Cloud metadata
	netip.MustParsePrefix("168.63.129.16/32"),   // Azure platform endpoint
	netip.MustParsePrefix("fd00:ec2::254/128"),  // AWS metadata over IPv6
}

// lookupAPICallHost resolves the target host. Overridden in tests.
var lookupAPICallHost = net.DefaultResolver.LookupNetIP

// vetAPICall checks a curl argv and returns the argv to run: the caller's options in long form,
// the single URL, and a fixed prefix that pins the connection to the address vetted here.
func vetAPICall(ctx context.Context, argv []string) ([]string, error) {
	opts, rawURL, err := vetCurlOptions(argv)
	if err != nil {
		return nil, err
	}
	target, err := parseAPICallURL(rawURL)
	if err != nil {
		return nil, err
	}
	addr, err := resolveAPICallHost(ctx, target.Hostname())
	if err != nil {
		return nil, err
	}
	return pinnedCurlArgv(opts, rawURL, addr, apiCallPort(target)), nil
}

func vetCurlOptions(argv []string) (opts []string, rawURL string, err error) {
	var urls []string
	take := func(name, value string) error {
		if err := curlLongOptions[name](value); err != nil {
			return fmt.Errorf("option %s: %w", name, err)
		}
		if name == "--url" {
			urls = append(urls, value)
			return nil
		}
		opts = append(opts, name, value)
		return nil
	}

	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		switch {
		case strings.HasPrefix(arg, "--"):
			check, ok := curlLongOptions[arg]
			if !ok {
				return nil, "", refuseCurlOption(arg)
			}
			if check == nil {
				opts = append(opts, arg)
				continue
			}
			if i+1 >= len(argv) {
				return nil, "", fmt.Errorf("option %s needs a value", arg)
			}
			i++
			if err := take(arg, argv[i]); err != nil {
				return nil, "", err
			}
		case len(arg) > 1 && arg[0] == '-':
			// Short options bundle (-sS), and the first one taking a value swallows the rest of
			// the word (-XPOST) or, when nothing is left, the next word.
			for j := 1; j < len(arg); j++ {
				name, ok := curlShortOptions[arg[j]]
				if !ok {
					return nil, "", refuseCurlOption("-" + string(arg[j]))
				}
				if curlLongOptions[name] == nil {
					opts = append(opts, name)
					continue
				}
				value := arg[j+1:]
				if value == "" {
					if i+1 >= len(argv) {
						return nil, "", fmt.Errorf("option -%c needs a value", arg[j])
					}
					i++
					value = argv[i]
				}
				if err := take(name, value); err != nil {
					return nil, "", err
				}
				break
			}
		default:
			urls = append(urls, arg)
		}
	}

	if len(urls) != 1 {
		return nil, "", fmt.Errorf("exactly one URL is required, got %d", len(urls))
	}
	return opts, urls[0], nil
}

func refuseCurlOption(name string) error {
	if curlRedirectOptions[name] {
		return fmt.Errorf("option %s is not allowed: redirects are not followed, read the Location header with -i and call that URL", name)
	}
	return fmt.Errorf("option %s is not allowed", name)
}

func parseAPICallURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid URL %q: %w", raw, err)
	}
	if s := strings.ToLower(u.Scheme); s != "http" && s != "https" {
		return nil, fmt.Errorf("URL %q: only http:// and https:// are allowed", raw)
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("URL %q has no host", raw)
	}
	if p := u.Port(); p != "" {
		if n, err := strconv.Atoi(p); err != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("URL %q has an invalid port", raw)
		}
	}
	return u, nil
}

func apiCallPort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	if strings.EqualFold(u.Scheme, "https") {
		return "443"
	}
	return "80"
}

// resolveAPICallHost refuses a host if any of its addresses is off limits, not just the first:
// a name answering with both a public and a metadata address would otherwise be a coin toss.
func resolveAPICallHost(ctx context.Context, host string) (netip.Addr, error) {
	if addr, err := netip.ParseAddr(host); err == nil {
		if isBlockedAPICallAddr(addr) {
			return netip.Addr{}, fmt.Errorf("address %s is not allowed (loopback, link-local or cloud metadata)", host)
		}
		return addr.Unmap(), nil
	}
	addrs, err := lookupAPICallHost(ctx, "ip", host)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("cannot resolve host %s: %w", host, err)
	}
	if len(addrs) == 0 {
		return netip.Addr{}, fmt.Errorf("cannot resolve host %s: no addresses", host)
	}
	for _, addr := range addrs {
		if isBlockedAPICallAddr(addr) {
			return netip.Addr{}, fmt.Errorf("host %s resolves to %s, which is not allowed (loopback, link-local or cloud metadata)", host, addr)
		}
	}
	return addrs[0].Unmap(), nil
}

func isBlockedAPICallAddr(addr netip.Addr) bool {
	addr = addr.Unmap().WithZone("")
	if addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() ||
		addr.IsInterfaceLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified() {
		return true
	}
	return slices.ContainsFunc(apiCallBlockedPrefixes, func(p netip.Prefix) bool { return p.Contains(addr) })
}

// pinnedCurlArgv puts the fixed prefix in front of the vetted options. -q must come first to skip
// ~/.curlrc. --connect-to with an empty host and port matches every connection, so curl dials
// the vetted address whatever it makes of the URL and whatever DNS answers the second time,
// while the Host header and TLS name still come from the URL.
func pinnedCurlArgv(opts []string, rawURL string, addr netip.Addr, port string) []string {
	host := addr.String()
	if addr.Is6() {
		host = "[" + host + "]"
	}
	argv := []string{
		"-q",
		"--proto", "=http,https",
		"--proto-redir", "=http,https",
		"--globoff",
		"--connect-to", "::" + host + ":" + port,
	}
	argv = append(argv, opts...)
	return append(argv, "--url", rawURL)
}

// curlOptionSummary lists the accepted options for the tool description, so it cannot drift
// from the table above.
func curlOptionSummary() (flags, valued string) {
	shortOf := make(map[string]byte, len(curlShortOptions))
	for c, name := range curlShortOptions {
		shortOf[name] = c
	}
	var plain, withValue []string
	for name, check := range curlLongOptions {
		label := name
		if c, ok := shortOf[name]; ok {
			label = "-" + string(c) + "/" + name
		}
		if check == nil {
			plain = append(plain, label)
		} else {
			withValue = append(withValue, label)
		}
	}
	slices.Sort(plain)
	slices.Sort(withValue)
	return strings.Join(plain, " "), strings.Join(withValue, " ")
}

func checkCurlAny(string) error { return nil }

func checkCurlNotFromFile(v string) error {
	if strings.HasPrefix(v, "@") {
		return errCurlValueFromFile
	}
	return nil
}

// checkCurlURLEncode follows curl's own reading: a leading '=' is literal content, otherwise
// whichever of '@' and '=' comes first decides between name@file and name=content.
func checkCurlURLEncode(v string) error {
	if strings.HasPrefix(v, "=") {
		return nil
	}
	if i := strings.IndexAny(v, "@="); i >= 0 && v[i] == '@' {
		return errCurlValueFromFile
	}
	return nil
}

// checkCurlCookie refuses a value without '=', which curl reads as a cookie file.
func checkCurlCookie(v string) error {
	if !strings.Contains(v, "=") {
		return errors.New("only name=value cookies are allowed, not a cookie file")
	}
	return nil
}

func checkCurlMethod(v string) error {
	if v == "" || strings.IndexFunc(v, func(r rune) bool { return (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') }) >= 0 {
		return fmt.Errorf("invalid method %q", v)
	}
	return nil
}

func checkCurlNumber(v string) error {
	if n, err := strconv.ParseFloat(v, 64); err != nil || n < 0 {
		return fmt.Errorf("invalid number %q", v)
	}
	return nil
}

// checkCurlWriteOut also refuses %output{file}, which since curl 8.3 redirects -w into a file.
func checkCurlWriteOut(v string) error {
	if err := checkCurlNotFromFile(v); err != nil {
		return err
	}
	if strings.Contains(strings.ToLower(v), "%output{") {
		return errors.New("%output{} is not allowed")
	}
	return nil
}

// checkCurlOutput lets through only the common "-o /dev/null -w %{http_code}" idiom.
func checkCurlOutput(v string) error {
	if v != "/dev/null" {
		return errors.New("writing to a file is not allowed, only /dev/null")
	}
	return nil
}
