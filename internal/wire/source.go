package wire

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
)

// Source identity: parsing hosted subfolder URLs and resolving refs.

// SourceRef is the parsed identity of what to fetch: hosting location,
// repository, reference as given in the URL, and subfolder path.
type SourceRef struct {
	Host      string
	Owner     string
	Repo      string
	Ref       string
	Subpath   string
	SourceURL string
}

// Display renders the canonical short form used in rows and errors.
// It never contains credentials: URLs with userinfo are rejected at parse.
func (s SourceRef) Display() string {
	return s.Host + "/" + s.Owner + "/" + s.Repo + "@" + s.Ref + ":" + s.Subpath
}

// CloneURL renders the repository clone URL backing this source.
func (s SourceRef) CloneURL() string {
	return "https://" + s.Host + "/" + s.Owner + "/" + s.Repo
}

// CacheKey identifies the upstream (repository plus requested ref) for
// per-run memoization. Two checkouts of one repo and ref share resolutions.
func (s SourceRef) CacheKey() string {
	return strings.Join([]string{s.Host, s.Owner, s.Repo, s.Ref}, "\x00")
}

// ShortSHA renders the display prefix of a commit SHA. Short or empty
// input passes through unchanged rather than panicking.
func ShortSHA(sha string) string {
	const shortLen = 7

	if len(sha) <= shortLen {
		return sha
	}

	return sha[:shortLen]
}

// ParseSourceURL parses a hosted subfolder URL of the shape
// https://{host}/{owner}/{repo}/tree/{ref}/{subpath...} into a SourceRef.
// The ref is the first segment after /tree/; refs containing slashes are
// disambiguated later by ResolveRef against the remote's ref listing.
func ParseSourceURL(raw string) (SourceRef, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return SourceRef{}, fmt.Errorf("empty URL: %w", ErrBadURL)
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return SourceRef{}, fmt.Errorf("parse %q: %w", trimmed, ErrBadURL)
	}

	if u.Scheme != "https" {
		return SourceRef{}, fmt.Errorf("scheme %q must be https: %w", u.Scheme, ErrBadURL)
	}

	if u.User != nil {
		return SourceRef{}, fmt.Errorf("URL must not embed credentials: %w", ErrBadURL)
	}

	host, err := parseHost(u.Host)
	if err != nil {
		return SourceRef{}, err
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 5 || parts[2] != "tree" {
		return SourceRef{}, fmt.Errorf("parse %q: %w", trimmed, ErrBadURL)
	}

	owner, err := checkName("owner", parts[0])
	if err != nil {
		return SourceRef{}, err
	}

	repo, err := checkName("repo", strings.TrimSuffix(parts[1], ".git"))
	if err != nil {
		return SourceRef{}, err
	}

	ref, err := checkName("ref", parts[3])
	if err != nil {
		return SourceRef{}, err
	}

	subpath, err := parseSubpath(parts[4:])
	if err != nil {
		return SourceRef{}, err
	}

	return SourceRef{Host: host, Owner: owner, Repo: repo, Ref: ref, Subpath: subpath, SourceURL: trimmed}, nil
}

// parseHost validates the URL authority: no port, no leading dot or dash,
// no whitespace or control characters.
func parseHost(hostport string) (string, error) {
	host := strings.ToLower(hostport)

	if host == "" || strings.ContainsAny(host, " \t\r\n") {
		return "", fmt.Errorf("bad host %q: %w", hostport, ErrBadURL)
	}

	if strings.Contains(host, ":") {
		return "", fmt.Errorf("host %q must not include a port: %w", hostport, ErrBadURL)
	}

	if strings.HasPrefix(host, "-") || strings.HasPrefix(host, ".") {
		return "", fmt.Errorf("bad host %q: %w", hostport, ErrBadURL)
	}

	if slices.Contains(strings.Split(host, "."), "") {
		return "", fmt.Errorf("bad host %q: %w", hostport, ErrBadURL)
	}

	return host, nil
}

// checkName validates one owner, repo, or ref segment: non-empty, not dot,
// no leading dash, no whitespace, control, or backslash characters.
func checkName(kind, seg string) (string, error) {
	if seg == "" || seg == "." || seg == ".." {
		return "", fmt.Errorf("empty %s: %w", kind, ErrBadURL)
	}

	if strings.HasPrefix(seg, "-") {
		return "", fmt.Errorf("bad %s %q: %w", kind, seg, ErrBadURL)
	}

	if strings.ContainsAny(seg, " \t\r\n\\") || strings.ContainsRune(seg, '\x7f') {
		return "", fmt.Errorf("bad %s %q: %w", kind, seg, ErrBadURL)
	}

	for _, r := range seg {
		if r < 0x20 {
			return "", fmt.Errorf("bad %s %q: %w", kind, seg, ErrBadURL)
		}
	}

	return seg, nil
}

// parseSubpath joins the remaining URL segments, rejecting empties and
// dot elements so the subpath can never escape its repository.
func parseSubpath(segs []string) (string, error) {
	for _, seg := range segs {
		if seg == "" || seg == "." || seg == ".." {
			return "", fmt.Errorf("bad subpath element %q: %w", seg, ErrBadURL)
		}

		if strings.ContainsAny(seg, " \t\r\n\\") {
			return "", fmt.Errorf("bad subpath element %q: %w", seg, ErrBadURL)
		}
	}

	return strings.Join(segs, "/"), nil
}

// IsPinnedSHA reports whether ref is a full 40-hex commit SHA, which needs
// no remote ref listing to resolve.
func IsPinnedSHA(ref string) bool {
	if len(ref) != 40 {
		return false
	}

	for _, r := range ref {
		isHex := r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F'
		if !isHex {
			return false
		}
	}

	return true
}

// ResolveRef disambiguates a greedily parsed ref/subpath pair against known
// branch and tag names (without refs/heads/ or refs/tags/ prefixes),
// supporting refs that contain slashes. It returns the actual ref and the
// remaining subpath, choosing the longest matching known name.
func ResolveRef(known []string, ref, subpath string) (actualRef, actualPath string, ok bool) {
	full := ref
	if subpath != "" {
		full += "/" + subpath
	}

	best := ""

	for _, k := range known {
		if k == "" {
			continue
		}

		if full == k || strings.HasPrefix(full, k+"/") {
			if len(k) > len(best) {
				best = k
			}
		}
	}

	if best == "" {
		return "", "", false
	}

	return best, strings.TrimPrefix(full[len(best):], "/"), true
}
