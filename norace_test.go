//go:build !race

package mdflow

// raceEnabled is false in an ordinary build; see the race-tagged twin.
const raceEnabled = false
