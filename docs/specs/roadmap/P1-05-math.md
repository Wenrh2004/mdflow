# [P1] Math: \(…\) / \[…\] delimiters and Pandoc `$` rules

Labels: P1, ai-native, bug

## Problem

- `cost $5 and $10` renders as inline math (`<code class="language-math">5 and </code>10`). Prices in prose are extremely common in model output.
- `\(a^2\)` and `\[ … \]` — the default delimiters of several major models — are not recognised; the backslashes are consumed as escapes.

## Proposal

- Apply Pandoc's `tex_math_dollars` rules: an opening `$` must be followed by a non-space; a closing `$` must be preceded by a non-space and not followed by a digit.
- Add opt-in `\(…\)` (inline) and `\[…\]` (display, may span lines) to `extension/math`.

## Acceptance

- Regression tests for currency text; golden tests for both delimiter styles, including streaming split points.
- Linear-work tests on unmatched delimiters (reuse `ParseWork`).
