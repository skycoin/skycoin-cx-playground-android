package programnetwork

import (
	"cmp"
	"slices"
	"sync"
	"time"
)

// QueuedHave is one have a node received, with the node that sent it.
type QueuedHave struct {
	Have
	From     string
	Received time.Time
}

// HaveQueue holds every have a node receives (#42) and shows them in the
// model of the CX net library, skycoin-cx#508 and its docs/Networking.md
// (#37). Haves that arrive between two updates wait. Update makes them one
// batch, sorted by sender and in arrival order within a sender, and the
// program reads the batch with ReadyCount and ReadyNext. Nothing the program
// can see changes between two updates, and nothing is dropped or ignored.
// Update, ReadyCount, ReadyNext and Clock belong to the one goroutine that
// runs the updates.
type HaveQueue struct {
	mu      sync.Mutex
	arrived []QueuedHave
	ready   chan struct{}
	start   time.Time
	timer   *time.Timer

	batch []QueuedHave
	next  int
	clock int64
}

func newHaveQueue() *HaveQueue {
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	return &HaveQueue{ready: make(chan struct{}, 1), start: time.Now(), timer: timer}
}

func (q *HaveQueue) push(h QueuedHave) {
	q.mu.Lock()
	q.arrived = append(q.arrived, h)
	q.mu.Unlock()
	select {
	case q.ready <- struct{}{}:
	default:
	}
}

// Update makes every have that arrived since the last update the new batch
// and returns its count. When nothing has arrived it waits up to wait for a
// have, and 0 does not wait. The haves of the batch before that were not
// read are gone, as in net.Update.
func (q *HaveQueue) Update(wait time.Duration) int {
	if wait > 0 && q.Pending() == 0 {
		q.timer.Reset(wait)
		select {
		case <-q.ready:
		case <-q.timer.C:
		}
		q.timer.Stop()
	}
	q.clock = time.Since(q.start).Milliseconds()
	q.mu.Lock()
	q.batch, q.arrived = q.arrived, q.batch[:0]
	q.mu.Unlock()
	q.next = 0
	// Sorted by sender as net.Update sorts by handle (skycoin-cx D49), stable
	// so the haves of one sender keep their arrival order.
	slices.SortStableFunc(q.batch, func(a, b QueuedHave) int { return cmp.Compare(a.From, b.From) })
	return len(q.batch)
}

// Pending is the number of haves that arrived since the last update.
func (q *HaveQueue) Pending() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.arrived)
}

// ReadyCount is the number of haves of the batch ReadyNext has not returned.
func (q *HaveQueue) ReadyCount() int { return len(q.batch) - q.next }

// ReadyNext returns the next have of the batch, or false after the last one.
func (q *HaveQueue) ReadyNext() (QueuedHave, bool) {
	if q.next >= len(q.batch) {
		return QueuedHave{}, false
	}
	q.next++
	return q.batch[q.next-1], true
}

// Clock is the reading of the last update, in milliseconds since the queue
// was made.
func (q *HaveQueue) Clock() int64 { return q.clock }

// Ready receives a value after haves arrived. It may also receive one for
// haves an update already took.
func (q *HaveQueue) Ready() <-chan struct{} { return q.ready }
