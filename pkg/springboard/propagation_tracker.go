package springboard

import (
	"container/heap"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"
)

type keyServerPair struct {
	key    string
	server string
}

func (ksp keyServerPair) Shorthand() string {
	return fmt.Sprintf("(%s...%s, %s)", ksp.key[0:8], ksp.key[len(ksp.key)-4:], ksp.server)
}

type relayInformation struct {
	board       Board
	destination string
	queuedAt    time.Time
	nextAttempt time.Time
	attempts    int
	index       int
}

func (ri relayInformation) lookupKey() keyServerPair {
	return keyServerPair{ri.board.Key, ri.destination}
}

type relayQueue struct {
	queue  []*relayInformation
	lookup map[keyServerPair]*relayInformation
}

func newRelayQueue() *relayQueue {
	return &relayQueue{
		lookup: map[keyServerPair]*relayInformation{},
	}
}

func (rq relayQueue) Len() int { return len(rq.queue) }

func (rq relayQueue) Less(i, j int) bool {
	return rq.queue[i].nextAttempt.Before(rq.queue[j].nextAttempt)
}

func (rq relayQueue) Swap(i, j int) {
	rq.queue[i], rq.queue[j] = rq.queue[j], rq.queue[i]
	rq.queue[i].index = i
	rq.queue[j].index = j
}

func (rq *relayQueue) Push(x any) {
	if rq.lookup == nil {
		rq.lookup = map[keyServerPair]*relayInformation{}
	}
	n := len(rq.queue)
	item := x.(*relayInformation)
	_, alreadHasQueuedItem := rq.lookup[item.lookupKey()]
	if alreadHasQueuedItem {
		log.Printf("RACE CONDITION: trying to Push %s, but it's already in the queue", item.lookupKey().Shorthand())
	}
	item.index = n
	rq.queue = append(rq.queue, item)
	rq.lookup[item.lookupKey()] = item
}

func (rq *relayQueue) Pop() any {
	old := rq.queue
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // avoid memory leak
	item.index = -1 // for safety
	rq.queue = old[0 : n-1]
	delete(rq.lookup, item.lookupKey())
	return item
}

func (rq relayQueue) LookUp(key string, server string) (info *relayInformation, found bool) {
	info, found = rq.lookup[keyServerPair{key, server}]
	return
}

func (rq relayQueue) AnyQueued() bool {
	return len(rq.queue) > 0
}

func (rq relayQueue) NextAttempt() time.Time {
	if len(rq.queue) > 0 {
		return rq.queue[0].nextAttempt
	} else {
		log.Println("RACE CONDITION: trying to get the next scheduled attempt when none are scheduled")
		return time.Time{}
	}
}

type propagationTracker struct {
	queue           *relayQueue
	mutex           *sync.Mutex
	bgThreadRunning bool
	fqdn            string
}

func newPropagationTracker(fqdn string) *propagationTracker {
	return &propagationTracker{
		queue: newRelayQueue(),
		mutex: &sync.Mutex{},
		fqdn:  fqdn,
	}
}

func (tracker *propagationTracker) Schedule(board Board, server string) {
	go func() {
		tracker.mutex.Lock()
		queuedItem, alreadyQueued := tracker.queue.LookUp(board.Key, server)
		if alreadyQueued {
			queuedItem.attempts = 0
			queuedItem.board = board
			queuedItem.queuedAt = time.Now()
			queuedItem.nextAttempt = time.Now().Add(5 * time.Minute)
			heap.Fix(tracker.queue, queuedItem.index)
			log.Printf("%s already queued, resetting the time to %s", queuedItem.lookupKey().Shorthand(), queuedItem.nextAttempt.Format(time.RFC3339))
		} else {
			newItem := &relayInformation{
				board:       board,
				destination: server,
				queuedAt:    time.Now(),
				nextAttempt: time.Now().Add(5 * time.Minute),
			}
			heap.Push(tracker.queue, newItem)
			log.Printf("%s queuing for propagation in 5 minutes (%s)", newItem.lookupKey().Shorthand(), newItem.nextAttempt.Format(time.RFC3339))
			if !tracker.bgThreadRunning {
				go tracker.processQueue()
			}
		}
		tracker.mutex.Unlock()
	}()
}

func (tracker *propagationTracker) processQueue() {
	tracker.mutex.Lock()
	if tracker.bgThreadRunning {
		log.Print("RACE CONDITION: tried to kick off the background thread with it already running")
		tracker.mutex.Unlock()
		return
	} else {
		log.Print("Queue processor thread spinning up")
		tracker.bgThreadRunning = true
		tracker.mutex.Unlock()
	}
	for true {
		tracker.mutex.Lock()
		if !tracker.queue.AnyQueued() {
			log.Print("Queue empty, processor thread spinning down")
			tracker.bgThreadRunning = false
			tracker.mutex.Unlock()
			return
		}
		if time.Now().After(tracker.queue.NextAttempt()) {
			nextUp := heap.Pop(tracker.queue).(*relayInformation)
			client := NewClient(nextUp.destination)
			logTag := nextUp.lookupKey().Shorthand()
			err := client.PostSignedBoard(nextUp.board)
			if err == nil {
				log.Printf("%s successfully propagated", logTag)
			} else {
				log.Printf("%s error posting board: %s", logTag, err.Error())
				nextUp.attempts++
				jitteredWait := rand.Intn(pow2(nextUp.attempts))
				if jitteredWait < 2 {
					jitteredWait = 2
				}
				nextUp.nextAttempt = time.Now().Add(time.Duration(jitteredWait) * time.Minute)
				if nextUp.nextAttempt.After(nextUp.queuedAt.Add(time.Hour)) {
					log.Printf("%s too many attempts, giving up", logTag)
				} else {
					log.Printf("%s will try again in %d minutes (%s)", logTag, jitteredWait, nextUp.nextAttempt.Format(time.RFC3339))
					heap.Push(tracker.queue, nextUp)
				}
			}
		}
		tracker.mutex.Unlock()
		time.Sleep(time.Second)
	}
}

func pow2(y int) (val int) {
	val = 1
	for i := 0; i < y; i++ {
		val *= 2
	}
	return
}
