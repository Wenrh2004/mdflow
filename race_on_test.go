//go:build race

package mdflow_test

// raceEnabled is true when the binary is built with the race detector, whose
// instrumentation cost does not scale linearly with input — so timing-ratio
// guards skip under it. -race runs still cover their correctness.
const raceEnabled = true
