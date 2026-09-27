package parser

import (
	"strings"

	"github.com/Wenrh2004/mdflow/internal/ascii"
)

// ---- lexical helpers (pure) ----

func runLength(s string, pos int, c byte) int {
	n := 0
	for pos+n < len(s) && s[pos+n] == c {
		n++
	}
	return n
}

type backtickRunPositions struct {
	starts []int
	next   int
}

// backtickScanCache is a lazy forward index of backtick runs. A failed opener
// may have to inspect the unresolved suffix, but later openers reuse the runs
// learned by that proof instead of rescanning the suffix. Per-length cursors
// discard cached positions monotonically, keeping lookup work linear too.
type backtickScanCache struct {
	byLength map[int]backtickRunPositions
	frontier int
}

func (c *backtickScanCache) reset() {
	c.frontier = 0
	clear(c.byLength)
}

func (c backtickScanCache) clone() backtickScanCache {
	cloned := backtickScanCache{frontier: c.frontier}
	if len(c.byLength) == 0 {
		return cloned
	}
	cloned.byLength = make(map[int]backtickRunPositions, len(c.byLength))
	for length, positions := range c.byLength {
		positions.starts = append([]int(nil), positions.starts...)
		cloned.byLength[length] = positions
	}
	return cloned
}

func (s *InlineState) findBacktickClose(from, length int) (int, bool) {
	cache := &s.backticks
	if positions, ok := cache.byLength[length]; ok {
		for positions.next < len(positions.starts) && positions.starts[positions.next] < from {
			positions.next++
			s.work++
		}
		cache.byLength[length] = positions
		if positions.next < len(positions.starts) {
			return positions.starts[positions.next], true
		}
	}

	scan := max(cache.frontier, from)
	for scan < len(s.src) {
		relative := strings.IndexByte(s.src[scan:], '`')
		if relative < 0 {
			s.work += len(s.src) - scan
			cache.frontier = len(s.src)
			return 0, false
		}
		start := scan + relative
		run := runLength(s.src, start, '`')
		s.work += relative + run
		if cache.byLength == nil {
			cache.byLength = make(map[int]backtickRunPositions)
		}
		positions := cache.byLength[run]
		positions.starts = append(positions.starts, start)
		cache.byLength[run] = positions
		cache.frontier = start + run
		if run == length {
			return start, true
		}
		scan = cache.frontier
	}
	return 0, false
}

func normalizeCodeSpan(s string) string {
	if strings.IndexByte(s, '\n') >= 0 {
		s = strings.ReplaceAll(s, "\n", " ")
	}
	if len(s) >= 2 && s[0] == ' ' && s[len(s)-1] == ' ' && strings.TrimSpace(s) != "" {
		s = s[1 : len(s)-1]
	}
	return s
}

func hasURIScheme(s string) bool {
	colon := strings.IndexByte(s, ':')
	if colon < 2 || colon > 32 || !ascii.IsAlpha(s[0]) {
		return false
	}
	for i := 1; i < colon; i++ {
		c := s[i]
		if !ascii.IsAlnum(c) && c != '+' && c != '.' && c != '-' {
			return false
		}
	}
	for i := colon + 1; i < len(s); i++ {
		c := s[i]
		if c <= 0x20 || c == 0x7f || c == '<' || c == '>' {
			return false
		}
	}
	return true
}

func isEmailLike(s string) bool {
	at := strings.IndexByte(s, '@')
	if at <= 0 || at == len(s)-1 || strings.IndexByte(s[at+1:], '@') >= 0 {
		return false
	}
	for i := 0; i < at; i++ {
		if !isEmailLocalByte(s[i]) {
			return false
		}
	}
	for start := at + 1; start < len(s); {
		end := strings.IndexByte(s[start:], '.')
		if end < 0 {
			end = len(s)
		} else {
			end += start
		}
		if !isEmailDomainLabel(s[start:end]) {
			return false
		}
		if end == len(s) {
			return true
		}
		start = end + 1
	}
	return false
}

func isEmailLocalByte(c byte) bool {
	if ascii.IsAlnum(c) {
		return true
	}
	switch c {
	case '.', '!', '#', '$', '%', '&', '\'', '*', '+', '/', '=', '?', '^', '_', '`', '{', '|', '}', '~', '-':
		return true
	default:
		return false
	}
}

func isEmailDomainLabel(s string) bool {
	if len(s) == 0 || len(s) > 63 || !ascii.IsAlnum(s[0]) || !ascii.IsAlnum(s[len(s)-1]) {
		return false
	}
	for i := 1; i < len(s)-1; i++ {
		if !ascii.IsAlnum(s[i]) && s[i] != '-' {
			return false
		}
	}
	return true
}

// bareLinkDestinationCache gives every possible unbracketed destination start
// its terminal byte (or noInlineIndex when an opening parenthesis is
// unmatched). It is built lazily at the first inline-link tail, then reused by
// later failed candidates so nested `](` suffixes cannot be rescanned.
