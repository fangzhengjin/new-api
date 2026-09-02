package common

import (
	"sync"
	"time"
)

type InMemoryRateLimiter struct {
	store              map[string]*[]rateLimitEntry
	mutex              sync.Mutex
	expirationDuration time.Duration
}

type rateLimitEntry struct {
	timestamp int64
	member    string
}

func (l *InMemoryRateLimiter) Init(expirationDuration time.Duration) {
	if l.store == nil {
		l.mutex.Lock()
		if l.store == nil {
			l.store = make(map[string]*[]rateLimitEntry)
			l.expirationDuration = expirationDuration
			if expirationDuration > 0 {
				go l.clearExpiredItems()
			}
		}
		l.mutex.Unlock()
	}
}

func (l *InMemoryRateLimiter) clearExpiredItems() {
	for {
		time.Sleep(l.expirationDuration)
		l.mutex.Lock()
		now := time.Now().Unix()
		for key := range l.store {
			queue := l.store[key]
			size := len(*queue)
			if size == 0 || now-(*queue)[size-1].timestamp > int64(l.expirationDuration.Seconds()) {
				delete(l.store, key)
			}
		}
		l.mutex.Unlock()
	}
}

// Request parameter duration's unit is seconds
func (l *InMemoryRateLimiter) Request(key string, maxRequestNum int, duration int64) bool {
	return l.Reserve(key, maxRequestNum, duration, "")
}

// Reserve records a request with a member that can be released later.
func (l *InMemoryRateLimiter) Reserve(key string, maxRequestNum int, duration int64, member string) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if maxRequestNum <= 0 {
		return true
	}
	now := time.Now().Unix()
	queue := l.activeQueueLocked(key, now, duration)
	if len(*queue) >= maxRequestNum {
		return false
	}
	*queue = append(*queue, rateLimitEntry{timestamp: now, member: member})
	return true
}

// Release removes a previously reserved request without affecting other entries.
func (l *InMemoryRateLimiter) Release(key string, member string) {
	if member == "" {
		return
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()
	queue, ok := l.store[key]
	if !ok {
		return
	}
	for index := range *queue {
		if (*queue)[index].member != member {
			continue
		}
		*queue = append((*queue)[:index], (*queue)[index+1:]...)
		if len(*queue) == 0 {
			delete(l.store, key)
		}
		return
	}
}

func (l *InMemoryRateLimiter) activeQueueLocked(key string, now int64, duration int64) *[]rateLimitEntry {
	queue, ok := l.store[key]
	if !ok {
		values := make([]rateLimitEntry, 0)
		l.store[key] = &values
		return &values
	}
	firstActive := 0
	for firstActive < len(*queue) && now-(*queue)[firstActive].timestamp >= duration {
		firstActive++
	}
	if firstActive > 0 {
		*queue = (*queue)[firstActive:]
	}
	return queue
}
