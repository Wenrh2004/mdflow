# [P2] Quality gates: differential fuzzing, OSS-Fuzz, coverage, perf regression

Labels: P2, testing

- Differential fuzzing against goldmark / cmark-gfm on normalised HTML — the way GFM example 201's deviation would have been found.
- OSS-Fuzz or ClusterFuzzLite for continuous fuzzing (the in-CI fuzz budget is 20 s per target).
- Coverage with `-coverpkg=./...` (package-local numbers understate `renderer/html` at 23%) and publish it.
- `benchstat` comparison on PRs against the base branch; keep wall-clock assertions out of unit tests (`TestParallelCrossover` is now opt-in via `MDFLOW_TIMING_TESTS=1`).
