//go:build race

package db

// The race detector slows code down 2-10x, so the timing budgets of the large
// tests do not apply under it.
const raceEnabled = true
