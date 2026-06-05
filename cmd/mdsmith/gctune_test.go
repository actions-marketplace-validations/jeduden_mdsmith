package main

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBatchGCTarget(t *testing.T) {
	// GOGC unset: apply the batch target so a short-lived check/fix run
	// stops collecting on every heap doubling.
	assert.Equal(t, batchGCPercent, batchGCTarget(""))

	// GOGC pinned by the user: leave the runtime default untouched.
	assert.Equal(t, -1, batchGCTarget("50"))
	assert.Equal(t, -1, batchGCTarget("off"))
	assert.Equal(t, -1, batchGCTarget("100"))
}

func TestTuneGCForBatch_SetsTargetWhenUnset(t *testing.T) {
	t.Setenv("GOGC", "") // treated as unset
	prev := debug.SetGCPercent(100)
	defer debug.SetGCPercent(prev)
	tuneGCForBatch()
	got := debug.SetGCPercent(100) // returns the value tuneGCForBatch set
	assert.Equal(t, batchGCPercent, got)
}

func TestTuneGCForBatch_RespectsExplicitGOGC(t *testing.T) {
	t.Setenv("GOGC", "50")
	prev := debug.SetGCPercent(123)
	defer debug.SetGCPercent(prev)
	tuneGCForBatch() // GOGC pinned ⇒ leaves the target untouched
	got := debug.SetGCPercent(123)
	assert.Equal(t, 123, got)
}
