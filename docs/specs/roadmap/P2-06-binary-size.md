# [P2] Binary size: the one dimension still behind gomark

Labels: P2, performance

`bench.TestBinarySize` (stripped, Go 1.26, linux/amd64): the core module adds ~456 KiB, `all` ~616 KiB, gomark ~328 KiB. The hardening PR already cut 124 KiB (dropped `net/url`, packed the entity table), and what remains is mostly the complete CommonMark implementation plus the Unicode case-fold table the spec requires — gomark passes 100/652 spec examples.

Options:

- Make the parallel and middleware render paths linker-eliminable (reach them through a function value set by `Workers`/`Transform`) so programs that never use them do not carry them.
- Compress the case-fold table (range encoding).
- Audit generic instantiations and `fmt` use in non-debug paths.
