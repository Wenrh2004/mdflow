# Roadmap issues

Drafted from the production-readiness review. Each file is one GitHub issue: the first line is the title, the `Labels:` line its labels. Create them with `scripts/open-roadmap-issues.sh` (needs `gh` with issues:write).

- [[P1] Streaming: incremental patch protocol with stable block IDs](P1-01-streaming-patch-protocol.md)
- [[P1] Streaming: speculative completion of unterminated inline syntax (anti-flicker)](P1-02-speculative-completion.md)
- [[P1] Source positions (byte offsets) on events and tokens](P1-03-source-positions.md)
- [[P1] CJK-friendly emphasis and soft line breaks](P1-04-cjk.md)
- [[P1] Math: \(…\) / \[…\] delimiters and Pandoc `$` rules](P1-05-math.md)
- [[P1] GFM completeness: spec suite, table row fix, autolinks, footnotes, alerts](P1-06-gfm-completeness.md)
- [[P1] LLM profile: reasoning tags, citations and a batteries-included preset](P1-07-llm-profile.md)
- [[P1] Structured extraction: code blocks, tables, RAG chunker, Markdown and mdast output](P1-08-extraction-apis.md)
- [[P1] Renderer: leaf-level overrides (code highlighting) and heading anchors](P1-09-renderer-hooks.md)
- [[P2] Shrink the public parser API before v1](P2-01-parser-api-surface.md)
- [[P2] Align remaining APIs with standard-library conventions](P2-02-stdlib-api-conventions.md)
- [[P2] Release engineering: first tag, changelog, compatibility gates, CI matrix](P2-03-release-engineering.md)
- [[P2] Quality gates: differential fuzzing, OSS-Fuzz, coverage, perf regression](P2-04-quality-gates.md)
- [[P2] Community and documentation](P2-05-community-docs.md)
- [[P2] Binary size: the one dimension still behind gomark](P2-06-binary-size.md)
