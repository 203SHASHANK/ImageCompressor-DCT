package dct

import (
	"runtime"
	"sync"
)

// plane is one image channel stored as a single row-major slice. One
// allocation per channel is both faster and far lighter on the garbage
// collector than a slice of row slices.
type plane struct {
	width, height int
	samples       []float64
}

func newPlane(width, height int) *plane {
	return &plane{width: width, height: height, samples: make([]float64, width*height)}
}

// row returns the samples of row r.
func (p *plane) row(r int) []float64 {
	return p.samples[r*p.width : (r+1)*p.width]
}

// ceilDiv returns a/b rounded up, for positive integers.
func ceilDiv(a, b int) int {
	return (a + b - 1) / b
}

// forEachRow calls fn(r) once for every r in [0, n), spreading the calls
// over a pool of worker goroutines, and returns when all calls are done.
//
// Each worker pulls row indices from a shared channel until it is closed.
// Handing out whole rows, rather than single pixels or blocks, keeps the
// channel traffic negligible next to the work done per item. fn must be safe
// to call concurrently for different rows; here every row writes only to its
// own part of the output, so no locking is needed.
func forEachRow(n int, fn func(r int)) {
	workers := runtime.GOMAXPROCS(0)
	if workers > n {
		workers = n
	}
	if workers <= 1 {
		for r := 0; r < n; r++ {
			fn(r)
		}
		return
	}

	rows := make(chan int, n)
	for r := 0; r < n; r++ {
		rows <- r
	}
	close(rows)

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for r := range rows {
				fn(r)
			}
		}()
	}
	wg.Wait()
}
