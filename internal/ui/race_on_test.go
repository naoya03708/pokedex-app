//go:build race

package ui_test

// slowdown scales time budgets under the race detector, which makes code
// several times slower.
const slowdown = 8
