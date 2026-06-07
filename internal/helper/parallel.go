package helper

import (
	"runtime"
	"sync"
	"sync/atomic"
)

const (
	minParallelWorkers   = 2
	maxIOParallelWorkers = 16
)

// RunBoundedParallel executes fn for each index with the default IO-oriented
// worker cap. Existing callers in the IO layer should generally use this.
func RunBoundedParallel(total int, fn func(i int)) {
	RunIOBoundedParallel(total, fn)
}

// RunIOBoundedParallel executes fn for each index with a worker cap tuned for
// blocking filesystem work.
func RunIOBoundedParallel(total int, fn func(i int)) {
	runParallel(total, ioWorkerLimit(total), fn)
}

// RunIOBoundedParallelWithLimit executes fn with IO-oriented worker sizing while
// respecting a caller-supplied upper bound when one is useful for memory-heavy
// operations.
func RunIOBoundedParallelWithLimit(total, maxWorkers int, fn func(i int)) {
	limit := ioWorkerLimit(total)
	if maxWorkers > 0 && maxWorkers < limit {
		limit = maxWorkers
	}
	runParallel(total, limit, fn)
}

// RunCPUBoundedParallel executes fn for each index with a worker cap tuned for
// CPU-bound work.
func RunCPUBoundedParallel(total int, fn func(i int)) {
	runParallel(total, cpuWorkerLimit(total), fn)
}

// RunIOBoundedParallelWhile executes fn with IO-oriented parallelism until all
// indices are processed or fn returns false for a worker iteration.
func RunIOBoundedParallelWhile(total int, fn func(i int) bool) {
	runParallelWhile(total, ioWorkerLimit(total), fn)
}

func runParallel(total, limit int, fn func(i int)) {
	if total <= 1 {
		for i := 0; i < total; i++ {
			fn(i)
		}
		return
	}

	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i := 0; i < total; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()
			fn(idx)
		}(i)
	}
	wg.Wait()
}

func runParallelWhile(total, workers int, fn func(i int) bool) {
	if total <= 0 {
		return
	}
	if total == 1 {
		_ = fn(0)
		return
	}

	var next atomic.Int64
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				idx := int(next.Add(1) - 1)
				if idx >= total {
					return
				}
				if !fn(idx) {
					return
				}
			}
		}()
	}
	wg.Wait()
}

func cpuWorkerLimit(total int) int {
	limit := runtime.GOMAXPROCS(0)
	if limit < minParallelWorkers {
		limit = minParallelWorkers
	}
	if limit > total {
		limit = total
	}
	return limit
}

func ioWorkerLimit(total int) int {
	limit := runtime.GOMAXPROCS(0) * 2
	if limit < minParallelWorkers {
		limit = minParallelWorkers
	}
	if limit > maxIOParallelWorkers {
		limit = maxIOParallelWorkers
	}
	if limit > total {
		limit = total
	}
	return limit
}
