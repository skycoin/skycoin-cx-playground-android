package programnetwork

import (
	"sync"
	"time"
)

// QueuedHave is one have a node received, with the node that sent it.
type QueuedHave struct {
	Have
	From     string
	Received time.Time
}

// HaveQueue holds every have a node receives, in arrival order, until the
// program reads it (#42). Nothing is dropped or ignored. Reading is one
// step, Read, so a deterministic update (#37) can take its place later.
type HaveQueue struct {
	mu    sync.Mutex
	items []QueuedHave
	ready chan struct{}
}

func newHaveQueue() *HaveQueue { return &HaveQueue{ready: make(chan struct{}, 1)} }

func (q *HaveQueue) push(h QueuedHave) {
	q.mu.Lock()
	q.items = append(q.items, h)
	q.mu.Unlock()
	select {
	case q.ready <- struct{}{}:
	default:
	}
}

// Read returns every queued have in arrival order and empties the queue.
func (q *HaveQueue) Read() []QueuedHave {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := q.items
	q.items = nil
	return out
}

// Len is the number of haves waiting.
func (q *HaveQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

// Ready receives a value after haves were queued.
func (q *HaveQueue) Ready() <-chan struct{} { return q.ready }
