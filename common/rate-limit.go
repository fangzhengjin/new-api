package common

import (
	"container/list"
	"sync"
	"time"
)

// rateLimitRequest stores one accepted request in a key's sliding window.
type rateLimitRequest struct {
	next      *rateLimitRequest
	timestamp int64
	member    string
}

// rateLimitQueue keeps accepted requests ordered from oldest to newest.
type rateLimitQueue struct {
	head   *rateLimitRequest
	tail   *rateLimitRequest
	length int
}

// append adds an accepted request without preallocating for the configured limit.
func (q *rateLimitQueue) append(timestamp int64, member string) {
	request := &rateLimitRequest{timestamp: timestamp, member: member}
	if q.tail == nil {
		q.head = request
		q.tail = request
	} else {
		q.tail.next = request
		q.tail = request
	}
	q.length++
}

// removeMember unlinks the oldest request carrying member, leaving other slots intact.
func (q *rateLimitQueue) removeMember(member string) {
	link := &q.head
	for node := q.head; node != nil; link = &node.next {
		if node.member != member {
			continue
		}
		*link = node.next
		if q.tail == node {
			q.tail = node.next
		}
		node.next = nil
		q.length--
		return
	}
}

// removeExpired releases every expired request at the front of the queue.
func (q *rateLimitQueue) removeExpired(now int64, duration int64) {
	if q.head == nil || now-q.head.timestamp < duration {
		return
	}

	// Requests are time ordered, so an expired tail means the whole queue expired.
	if now-q.tail.timestamp >= duration {
		q.clear()
		return
	}

	for now-q.head.timestamp >= duration {
		expired := q.head
		q.head = expired.next
		expired.next = nil
		q.length--
	}
}

// clear releases the whole request chain when its key expires.
func (q *rateLimitQueue) clear() {
	q.head = nil
	q.tail = nil
	q.length = 0
}

// rateLimitEntry is both a rate-limit bucket and a node in the key-level LRU.
type rateLimitEntry struct {
	lastActive time.Time
	element    *list.Element
	requests   rateLimitQueue
	key        string
}

// InMemoryRateLimiter implements a sliding-window limiter with idle-key eviction.
type InMemoryRateLimiter struct {
	store              map[string]*rateLimitEntry
	lru                *list.List
	mutex              sync.Mutex
	expirationDuration time.Duration
}

// Init initializes the limiter once. Repeated calls leave the first configuration unchanged.
func (l *InMemoryRateLimiter) Init(expirationDuration time.Duration) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.store != nil {
		return
	}

	l.store = make(map[string]*rateLimitEntry)
	l.lru = list.New()
	l.expirationDuration = expirationDuration
	if expirationDuration > 0 {
		go l.clearExpiredItems(time.NewTicker(expirationDuration).C)
	}
}

// clearExpiredItems periodically removes expired entries from the LRU tail.
func (l *InMemoryRateLimiter) clearExpiredItems(ticks <-chan time.Time) {
	for now := range ticks {
		l.deleteExpiredEntries(now)
	}
}

// deleteExpiredEntries walks only the oldest LRU entries and stops at the first active key.
func (l *InMemoryRateLimiter) deleteExpiredEntries(now time.Time) {
	if l.expirationDuration <= 0 {
		return
	}

	l.mutex.Lock()
	defer l.mutex.Unlock()

	for {
		oldest := l.lru.Back()
		if oldest == nil {
			return
		}

		entry := oldest.Value.(*rateLimitEntry)
		if now.Sub(entry.lastActive) < l.expirationDuration {
			return
		}

		delete(l.store, entry.key)
		l.lru.Remove(oldest)
		entry.element = nil
		entry.requests.clear()
	}
}

// entryLocked returns the key's bucket, creating it when absent, and drops requests
// that already left the sliding window. Callers must hold the limiter mutex.
func (l *InMemoryRateLimiter) entryLocked(key string, now time.Time, duration int64) *rateLimitEntry {
	entry, ok := l.store[key]
	if !ok {
		entry = &rateLimitEntry{
			key:        key,
			lastActive: now,
		}
		entry.element = l.lru.PushFront(entry)
		l.store[key] = entry
		return entry
	}
	entry.requests.removeExpired(now.Unix(), duration)
	entry.lastActive = now
	l.lru.MoveToFront(entry.element)
	return entry
}

// Request parameter duration's unit is seconds
func (l *InMemoryRateLimiter) Request(key string, maxRequestNum int, duration int64) bool {
	return l.Reserve(key, maxRequestNum, duration, "")
}

// Reserve consumes one slot of the key's sliding window and attributes it to member,
// so the caller can release exactly that slot later. An empty member keeps plain
// Request semantics and the slot can then never be released individually. A limit
// that is not positive denies every request, keeping the upstream window rule: a
// disabled dimension must skip this call instead of passing zero.
func (l *InMemoryRateLimiter) Reserve(key string, maxRequestNum int, duration int64, member string) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	now := time.Now()
	entry := l.entryLocked(key, now, duration)

	allowed := entry.requests.length < maxRequestNum
	if allowed {
		entry.requests.append(now.Unix(), member)
	}

	return allowed
}

// Release removes a previously reserved request without affecting other entries.
func (l *InMemoryRateLimiter) Release(key string, member string) {
	if member == "" {
		return
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()

	entry, ok := l.store[key]
	if !ok {
		return
	}
	entry.requests.removeMember(member)
}
