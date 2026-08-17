package pool

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMapReturnsResultsInInputOrderRegardlessOfCompletionOrder(t *testing.T) {
	in := make([]int, 64)
	for index := range in {
		in[index] = index
	}

	// Later entries finish first, so a pool that inherited completion order would come back reversed.
	got := Map(8, in, func(value int) int {
		time.Sleep(time.Duration(len(in)-value) * time.Microsecond * 200)
		return value * 2
	})

	if len(got) != len(in) {
		t.Fatalf("got %d results, want %d", len(got), len(in))
	}
	for index, value := range got {
		if value != index*2 {
			t.Fatalf("result %d is %d, want %d", index, value, index*2)
		}
	}
}

func TestMapNeverRunsMoreThanTheLimitAtOnce(t *testing.T) {
	var running, peak atomic.Int64

	in := make([]int, 200)
	Map(4, in, func(int) int {
		current := running.Add(1)
		for {
			high := peak.Load()
			if current <= high || peak.CompareAndSwap(high, current) {
				break
			}
		}
		time.Sleep(time.Millisecond)
		running.Add(-1)
		return 0
	})

	if peak.Load() > 4 {
		t.Errorf("peak concurrency is %d, want at most 4", peak.Load())
	}
	if peak.Load() < 2 {
		t.Errorf("peak concurrency is %d, so the work never overlapped", peak.Load())
	}
}

func TestMapSurvivesAPanickingWorkerAndYieldsItsZeroValue(t *testing.T) {
	in := []int{0, 1, 2, 3, 4, 5, 6, 7}

	got := Map(3, in, func(value int) string {
		if value == 3 {
			panic("this repo exploded")
		}
		return "ok"
	})

	want := []string{"ok", "ok", "ok", "", "ok", "ok", "ok", "ok"}
	if len(got) != len(want) {
		t.Fatalf("got %d results, want %d", len(got), len(want))
	}
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("result %d is %q, want %q", index, got[index], want[index])
		}
	}
}

func TestMapReturnsNothingForNoInputAndStillRunsWithANonsenseLimit(t *testing.T) {
	if got := Map(8, nil, func(int) int { return 1 }); got != nil {
		t.Errorf("got %v for no input, want nil", got)
	}

	got := Map(0, []int{1, 2, 3}, func(value int) int { return value })
	if len(got) != 3 || got[0] != 1 || got[2] != 3 {
		t.Errorf("got %v with a zero limit, want [1 2 3]", got)
	}
}

func TestMapRunsEveryInputExactlyOnce(t *testing.T) {
	var mu sync.Mutex
	seen := map[int]int{}

	in := make([]int, 500)
	for index := range in {
		in[index] = index
	}
	Map(Limit, in, func(value int) int {
		if rand.Intn(4) == 0 {
			time.Sleep(time.Microsecond)
		}
		mu.Lock()
		seen[value]++
		mu.Unlock()
		return value
	})

	for _, value := range in {
		if seen[value] != 1 {
			t.Fatalf("input %d ran %d times, want 1", value, seen[value])
		}
	}
}

func TestDetachedReturnsTheWorkersResultOnce(t *testing.T) {
	wait := Detached(func() string {
		time.Sleep(5 * time.Millisecond)
		return "fetched"
	})

	if got := wait(); got != "fetched" {
		t.Errorf("got %q, want %q", got, "fetched")
	}
	if got := wait(); got != "fetched" {
		t.Errorf("second wait got %q, want %q", got, "fetched")
	}
}

func TestDetachedRaisesTheWorkersPanicOnTheWaitingGoroutine(t *testing.T) {
	wait := Detached(func() []string {
		panic("gh exploded")
	})

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("waiting returned normally, so the panic was swallowed")
		}
		if got, ok := recovered.(string); !ok || got != "gh exploded" {
			t.Errorf("recovered %v, want %q", recovered, "gh exploded")
		}
	}()
	wait()
}
