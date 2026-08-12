# T04 — Opt-in URI-scheme allowlist for link/image destinations

- Size: M
- Severity: HIGH (most likely real-world XSS an integrator hits)
- Source: security review (V2, measured)
- User stories: 10, 11, 12, 13, 14

## Blocking edges
- Blocked by: —
- Blocks: —

## The finding
`renderer/html/html.go` emits link `href` and image `src` verbatim; destinations are stored unfiltered in `parser/inline.go` (inline/reference and autolink). There is no scheme allowlist/denylist anywhere. Observed on default `mdflow.New()`:

```
[click](javascript:alert(1))        -> <a href="javascript:alert(1)">click</a>
[x](java&#115;cript:alert(1))       -> <a href="javascript:alert(1)">x</a>   (entity-obfuscated 's')
[x](&#106;avascript:alert(1))       -> <a href="javascript:alert(1)">x</a>   (entity 'j')
[x](javascript&#58;alert(1))        -> <a href="javascript:alert(1)">x</a>   (entity ':')
![img](javascript:alert(1))         -> <img src="javascript:alert(1)" alt="img" />
<javascript:alert(1)>               -> <a href="javascript:alert(1)">…</a>   (autolink)
[x](data:text/html,<script>…)       -> <a href="data:text/html,%3Cscript%3E…">x</a>
[x](vbscript:msgbox(1))             -> <a href="vbscript:msgbox(1)">x</a>
```

This is spec-compliant (cmark does not filter either) and correctly scoped in the README to raw-HTML escaping — **not** a contract violation. But the entity-obfuscated variants are the sharp edge: the parser decodes `java&#115;cript:` to `javascript:` *before* it reaches the href, so a downstream sanitizer that only string-matches source misses it.

## What done looks like
- [ ] An opt-in option (mirroring `WithUnsafeHTML`) filters link/image destinations against a scheme allowlist: default `http`, `https`, `mailto`, `tel`, relative/fragment/`?`-relative.
- [ ] The check runs on the **decoded** destination (after entity resolution + percent-normalization), so entity-obfuscated schemes are caught.
- [ ] `data:` navigation (at least `data:text/html`) is blocked by default; disallowed destinations render inert (empty / `#`) but keep the link text.
- [ ] Default `mdflow.New()` output is byte-for-byte unchanged (option strictly opt-in); conformance stays 652/652.
- [ ] README + `doc.go` state that safe-by-default covers raw HTML only, URI schemes are unfiltered unless opted in, and destination entities are decoded pre-output. **This doc note ships even if the option slips** — it is the documentation fallback the scope decision allows.

## Seam & tests
Seam #1 (`HTML(string) → string`). Add to `renderer/html/html_test.go` / `uri_normalization_test.go`: with the option on, each vector above (including all three entity-obfuscated forms and the autolink) renders inert; with the option off, output matches today's bytes. Assert the rendered string, not internal destination state.

## Notes
Place the filter at the renderer boundary where the decoded destination already exists — not the source-scan layer — so obfuscation cannot route around it.
