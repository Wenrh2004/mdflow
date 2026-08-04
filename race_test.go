//go:build race

package mdflow

// raceEnabled is true when the binary is built with the race detector. The
// detector serialises goroutine scheduling enough to erase the fan-out's
// speedup, so timing-sensitive parallelism guards skip under it.
const raceEnabled = true
