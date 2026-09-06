package whisper

import "testing"

func TestDefaultThreadsWithinBounds(t *testing.T) {
	got := DefaultThreads()
	if got < minThreads || got > maxThreads {
		t.Fatalf("thread count %d outside [%d, %d]", got, minThreads, maxThreads)
	}
}

func TestDefaultThreadsNeverExceedsCores(t *testing.T) {
	total, _, _ := CoreSummary()
	if got := DefaultThreads(); got > total {
		t.Fatalf("asked for %d threads on a %d-core machine", got, total)
	}
}
