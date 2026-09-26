# [P2] Shrink the public parser API before v1

Labels: P2, api

## Problem

`parser.BlockState` exports ~25 driver-internal methods (`FeedLine`, `CollectAll`, `HeldCount`, `ReleaseEvents`, `SealReferences`, `ParseInlineFinal`, `ReferencesRefused`, `AppendInline`, …). Freezing them at v1 makes every internal change breaking.

## Proposal

- Move driver internals to `internal/`; keep only what extension rules need (`StartAccumulator`, `AddContinuation`, `AddFinalise`, `PromoteParagraph`, `Emit*`, the `InlineState` surface).
- Settle the open design tickets first: T08 (deferred document-state seam), T09 (single paragraph-interrupt interface), T10 (split `block.go`/`inline.go`), T11 (`PromoteParagraph` ref-defs), T14 (`Workers(0)` default), T17.
- Gate with `apidiff`/`gorelease` in CI once v0.1.0 is tagged.
