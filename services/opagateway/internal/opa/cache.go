package opa

import (
	"sync"
	"time"
)

type cacheItem struct {
	val       Decision
	expiresAt time.Time
}

type DecisionCache struct {
	mu       sync.Mutex
	items    map[string]cacheItem
	ttl      time.Duration
	maxItems int
}

func NewDecisionCache(ttl time.Duration, maxItems int) *DecisionCache {
	if ttl <= 0 {
		ttl = 3 * time.Second
	}
	if maxItems <= 0 {
		maxItems = 5000
	}
	return &DecisionCache{
		items:    make(map[string]cacheItem, maxItems),
		ttl:      ttl,
		maxItems: maxItems,
	}
}

func (c *DecisionCache) Get(key string) (Decision, bool) {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	it, ok := c.items[key]
	if !ok {
		return Decision{}, false
	}
	if now.After(it.expiresAt) {
		delete(c.items, key)
		return Decision{}, false
	}
	return it.val, true
}

func (c *DecisionCache) Put(key string, v Decision) {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	// naive eviction: if too big, clear (good enough for MVP CI)
	if len(c.items) >= c.maxItems {
		c.items = make(map[string]cacheItem, c.maxItems)
	}
	c.items[key] = cacheItem{val: v, expiresAt: now.Add(c.ttl)}
}