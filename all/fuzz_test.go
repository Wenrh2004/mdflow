package all_test

import (
	"strings"
	"testing"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/all"
	"github.com/Wenrh2004/mdflow/extension/rawhtml"
)

// FuzzAllProfiles runs every bundled flavour — the configuration a batteries-
// included user actually ships — through the batch, streaming and trusted-HTML
// paths. It asserts no panic, byte-identical streaming, bounded output, and
// that the safe profile never lets a raw <script tag through.
func FuzzAllProfiles(f *testing.F) {
	for _, s := range []string{
		"| a | b |\n|---|---|\n| 1 | 2 |\n",
		"- [x] done\n- [ ] todo\n",
		"~~strike~~ ==mark== ^sup^ ~sub~ ||spoiler||\n",
		"$x^2$ and\n\n$$\na\n$$\n",
		"#tag [[wiki]] ![[embed]]\n",
		"<script>alert(1)</script>\n",
		"[x]: /u\n\n[x][x]\n",
	} {
		f.Add(s, 3)
	}
	safe := all.New()
	trusted := all.New(rawhtml.WithUnsafeHTML())
	f.Fuzz(func(t *testing.T, src string, chunk int) {
		got := safe.HTML(src)
		if limit := 64*len(src) + 256<<10; len(got) > limit {
			t.Fatalf("output %d bytes from %d bytes of input exceeds %d\n src: %q", len(got), len(src), limit, src)
		}
		if strings.Contains(strings.ToLower(got), "<script") {
			t.Fatalf("safe profile emitted a raw <script tag\n src: %q\n out: %q", src, got)
		}
		if chunk <= 0 {
			chunk = 1
		}
		if streamed := stream(safe, src, chunk); streamed != got {
			t.Fatalf("stream (chunk %d) differs from HTML\n src: %q\n  got: %q\n want: %q", chunk, src, streamed, got)
		}
		_ = trusted.HTML(src)
	})
}

func stream(p *mdflow.Parser, src string, chunk int) string {
	var b strings.Builder
	s := p.Stream()
	for len(src) > 0 {
		n := min(chunk, len(src))
		b.WriteString(s.Feed(src[:n]))
		_ = s.Provisional()
		src = src[n:]
	}
	b.WriteString(s.Close())
	return b.String()
}
