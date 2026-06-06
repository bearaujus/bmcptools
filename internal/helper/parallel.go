package helper

import (
	"runtime"
	"sync"
)

const maxParallelWorkers = 8

// RunBoundedParallel executes fn for each index with a conservative worker cap.
func RunBoundedParallel(total int, fn func(i int)) {
	if total <= 1 {
		for i := 0; i < total; i++ {
			fn(i)
		}
		return
	}

	limit := runtime.GOMAXPROCS(0)
	if limit < 2 {
		limit = 2
	}
	if limit > maxParallelWorkers {
		limit = maxParallelWorkers
	}
	if limit > total {
		limit = total
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
