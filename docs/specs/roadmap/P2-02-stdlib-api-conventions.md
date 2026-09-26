# [P2] Align remaining APIs with standard-library conventions

Labels: P2, api

Done in the hardening PR: `Parser.With` (options in a chain), `Parser.NewWriter` (an `io.WriteCloser` like `gzip.Writer`), `ErrClosed`, `BlockState.AppendInline` (append-style), `Spec()` returning a copy, `HTMLParallel`/`RenderParallel` deprecated.

Remaining, breaking, so best done before v1:

- `Stream.Close() string` shadows the `io.Closer` shape; consider `Stream.Flush() string` / `Finish`, leaving `Close() error` semantics to `Writer`.
- `Stream.Write` implements `io.Writer` but discards output — surprising; remove it now that `NewWriter` exists.
- `Stream.Blocks()` (a count) collides in name with `Parser.Blocks()` (an iterator); rename to `Stream.BlockCount()`.
- `token.Tag` is a process-global `uint8` registry keyed by name: two third-party extensions choosing the same name silently share a tag, and 255 is a hard cap that panics. Namespace names (module path) and widen to `uint16`.
- `Event` is 136 bytes passed by value through `iter.Seq`; consider moving rarely used fields behind accessors.
- Remove the deprecated `HTMLParallel`/`RenderParallel` before v1.
- `iterx.Collect` duplicates `slices.Collect`; deprecate in favour of the standard library.
