package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type Cache struct {
	mu         sync.Mutex
	entries    map[string]*entry
	maxEntries int
	ttl        time.Duration
	newLimiter func() *rate.Limiter
}

type entry struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

func NewCache(maxEntries int, ttl time.Duration, newLimiter func() *rate.Limiter) *Cache {
	if maxEntries <= 0 {
		maxEntries = 10000
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &Cache{
		entries:    make(map[string]*entry, maxEntries),
		maxEntries: maxEntries,
		ttl:        ttl,
		newLimiter: newLimiter,
	}
}

func (c *Cache) Get(ip string, now time.Time) *rate.Limiter {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.sweepLocked(now)
	if e, ok := c.entries[ip]; ok {
		e.lastSeen = now
		return e.lim
	}
	if len(c.entries) >= c.maxEntries {
		// evict oldest
		var oldestIP string
		var oldest time.Time
		first := true
		for k, v := range c.entries {
			if first || v.lastSeen.Before(oldest) {
				oldestIP = k
				oldest = v.lastSeen
				first = false
			}
		}
		delete(c.entries, oldestIP)
	}
	lim := c.newLimiter()
	c.entries[ip] = &entry{lim: lim, lastSeen: now}
	return lim
}

func (c *Cache) Size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

func (c *Cache) sweepLocked(now time.Time) {
	cutoff := now.Add(-c.ttl)
	for ip, e := range c.entries {
		if e.lastSeen.Before(cutoff) {
			delete(c.entries, ip)
		}
	}
}
