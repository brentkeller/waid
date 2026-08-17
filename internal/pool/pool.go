// Package pool runs bounded concurrent work whose results keep the order of their inputs. Detection
// is subprocess-bound rather than CPU-bound, and its output has to be identical run to run, so the
// two properties that matter are a cap on how many processes are in flight and an ordering that is
// never inherited from completion.
package pool

import "sync"

// Limit is how many probes run at once. The constraint is process spawn overhead rather than cores,
// so it is a fixed number rather than one derived from NumCPU.
const Limit = 16

// Map applies work to every input, at most limit at a time, and returns the results in input order.
// A worker that panics yields its input's zero value and leaves the rest of the pool running,
// because one unprobeable repo is a repo with nothing to report, not a failed scan.
func Map[In, Out any](limit int, in []In, work func(In) Out) []Out {
	if len(in) == 0 {
		return nil
	}
	if limit < 1 {
		limit = 1
	}

	out := make([]Out, len(in))
	slots := make(chan struct{}, limit)
	var running sync.WaitGroup

	for index, input := range in {
		running.Add(1)
		slots <- struct{}{}
		go func() {
			defer running.Done()
			defer func() { <-slots }()
			defer func() { _ = recover() }()
			out[index] = work(input)
		}()
	}
	running.Wait()
	return out
}

// Detached starts work on its own goroutine and returns the function that waits for it. Moving the
// work off the caller's goroutine is all it does: a panic is carried across and raised again on the
// waiting goroutine, so a detached call fails exactly where the same call made inline would. Waiting
// more than once is safe.
func Detached[Out any](work func() Out) func() Out {
	var out Out
	var failure any
	done := make(chan struct{})

	go func() {
		defer close(done)
		defer func() { failure = recover() }()
		out = work()
	}()

	return func() Out {
		<-done
		if failure != nil {
			panic(failure)
		}
		return out
	}
}
