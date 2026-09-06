package whisper

import (
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// Thread count bounds. Below four the engine leaves obvious performance on the
// table; above eight synchronisation overhead starts to dominate.
const (
	minThreads = 4
	maxThreads = 8
)

// DefaultThreads picks a thread count for this machine.
//
// Measured on an M4 Pro (10 performance + 4 efficiency cores) with
// large-v3-turbo on a 12s Russian clip:
//
//	 6 threads  4.78s
//	 8 threads  3.39s  <- best
//	10 threads  4.11s
//	14 threads  worse still
//
// More threads is not better: the model waits on its slowest worker, so
// scheduling onto efficiency cores hurts, and past a point the synchronisation
// cost outweighs the extra parallelism. Leaving two performance cores free for
// the rest of the system reproduced the measured optimum, so that is the rule.
func DefaultThreads() int {
	cores := performanceCores()
	if cores <= 0 {
		cores = runtime.NumCPU()
	}

	threads := cores - 2
	if threads > maxThreads {
		threads = maxThreads
	}
	if threads < minThreads {
		threads = minThreads
	}
	// Never ask for more threads than the machine has cores.
	if threads > cores {
		threads = cores
	}
	return threads
}

// performanceCores returns the number of performance cores on Apple Silicon.
// It reports 0 on Intel Macs and anywhere else the key is missing, letting the
// caller fall back to the total core count.
func performanceCores() int {
	out, err := exec.Command("sysctl", "-n", "hw.perflevel0.logicalcpu").Output()
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0
	}
	return n
}

// CoreSummary describes the CPU layout for diagnostics.
func CoreSummary() (total, performance, efficiency int) {
	total = runtime.NumCPU()
	performance = performanceCores()
	if performance > 0 {
		efficiency = total - performance
	}
	return total, performance, efficiency
}
