package mdflow_test

import (
	"fmt"
	"os"
	"strings"

	"github.com/Wenrh2004/mdflow"
	"github.com/Wenrh2004/mdflow/extension"
	"github.com/Wenrh2004/mdflow/iterx"
	"github.com/Wenrh2004/mdflow/renderer"
	"github.com/Wenrh2004/mdflow/token"
)

func ExampleHTML() {
	fmt.Print(mdflow.HTML("# Title\n\nSome *markdown*.\n"))
	// Output:
	// <h1>Title</h1>
	// <p>Some <em>markdown</em>.</p>
}

func ExampleParser_Transform() {
	// Each call returns a new Parser; `mdflow.New()` is untouched.
	p := mdflow.New().
		Transform(mdflow.ShiftHeadings(1)).
		Transform(mdflow.Unwrap(mdflow.IsLink))

	fmt.Print(p.HTML("# Docs\n\nSee [the guide](https://example.com).\n"))
	// Output:
	// <h2>Docs</h2>
	// <p>See the guide.</p>
}

func ExampleParser_Render() {
	// Render streams into any io.Writer without building the whole string.
	_ = mdflow.New().Render(os.Stdout, "- a\n- b\n")
	// Output:
	// <ul>
	// <li>a</li>
	// <li>b</li>
	// </ul>
}

func ExampleParser_Stream() {
	// An LLM-style producer: chunks arrive at arbitrary boundaries.
	s := mdflow.New().Stream()
	var out strings.Builder
	for _, chunk := range []string{"# Str", "eaming\n\nfir", "st para\n\nsecond"} {
		out.WriteString(s.Feed(chunk))
	}
	out.WriteString(s.Close())

	fmt.Print(out.String())
	// Output:
	// <h1>Streaming</h1>
	// <p>first para</p>
	// <p>second</p>
}

func ExampleParser_Stream_provisional() {
	s := mdflow.New().Stream()
	s.Feed("> a quote still being ty")
	// Nothing has closed yet, but the tail is already displayable.
	fmt.Print(s.Provisional())
	// Output:
	// <blockquote>
	// <p>a quote still being ty</p>
	// </blockquote>
}

func ExampleParser_Headings() {
	src := "# Intro\n\ntext\n\n## Setup\n\n### Requirements\n"
	for _, h := range mdflow.New().Headings(src) {
		fmt.Printf("%d %s (#%s)\n", h.Level, h.Text, mdflow.Slugify(h.Text))
	}
	// Output:
	// 1 Intro (#intro)
	// 2 Setup (#setup)
	// 3 Requirements (#requirements)
}

func ExampleParser_Text() {
	src := "# Title\n\nSome **bold** words.\n\n```go\nnotIndexed := 1\n```\n"
	fmt.Printf("%q\n", mdflow.New().Text(src))
	// Output:
	// "Title\n\nSome bold words."
}

func ExampleReduce() {
	src := "# Title\n\nfive plain words right here\n"

	words := iterx.Reduce(mdflow.Events(src), 0, func(n int, e mdflow.Event) int {
		if e.Type == mdflow.TextEvent {
			return n + len(strings.Fields(e.Text))
		}
		return n
	})
	fmt.Println(words)
	// Output: 6
}

func ExampleFilterMap() {
	src := "See [go](https://go.dev) and [rust](https://rust-lang.org).\n"

	links := iterx.Collect(iterx.FilterMap(mdflow.Events(src),
		func(e mdflow.Event) (string, bool) {
			if e.Type == mdflow.EnterEvent && e.Node == token.Link {
				return e.Dest, true
			}
			return "", false
		}))
	fmt.Println(links)
	// Output: [https://go.dev https://rust-lang.org]
}

// A capability pairs syntax with the output it produces. This one changes only
// output, so it gives no Syntax half — it restyles a node the core already
// parses.
func Example_capability() {
	highlight := extension.Capability{
		Name: "highlighted-code",
		Output: func(r renderer.Renderer) {
			renderer.OverrideNode(r, token.CodeSpan, func(w renderer.Writer, n token.Inline) {
				w.WriteString(`<code class="hl">` + n.Text + "</code>")
			})
		},
	}

	fmt.Print(mdflow.New().WithExtensions(highlight).HTML("call `run()` now\n"))
	// Output:
	// <p>call <code class="hl">run()</code> now</p>
}

func ExampleParser_Workers() {
	// Fan the inline phase across cores. Output is byte-identical to the
	// sequential path; only large documents take the fan-out.
	md := mdflow.New().Workers(4)
	fmt.Print(md.HTML("# Small doc\n\nfalls back to sequential.\n"))
	// Output:
	// <h1>Small doc</h1>
	// <p>falls back to sequential.</p>
}
