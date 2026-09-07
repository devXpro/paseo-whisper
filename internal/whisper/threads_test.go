package whisper

import "testing"

func TestDefaultThreadsRespectsBoundsAndCores(t *testing.T) {
	total, _, _ := CoreSummary()
	got := DefaultThreads()

	// The core count outranks the lower bound: asking for four threads on a
	// two-core runner would be worse than asking for two.
	floor := minThreads
	if total < floor {
		floor = total
	}
	ceiling := maxThreads
	if total < ceiling {
		ceiling = total
	}

	if got < floor || got > ceiling {
		t.Fatalf("thread count %d outside [%d, %d] on a %d-core machine", got, floor, ceiling, total)
	}
}

func TestDefaultThreadsNeverExceedsCores(t *testing.T) {
	total, _, _ := CoreSummary()
	if got := DefaultThreads(); got > total {
		t.Fatalf("asked for %d threads on a %d-core machine", got, total)
	}
}

func TestDefaultThreadsIsPositive(t *testing.T) {
	if got := DefaultThreads(); got < 1 {
		t.Fatalf("thread count must be at least 1, got %d", got)
	}
}
