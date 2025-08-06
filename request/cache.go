package request

import (
	"container/list"
	"sync"
)

// Cache is a thread-safe cache for storing resource names
type Cache struct {
	mu    sync.Mutex
	// TODO: add cap and drop oldest item if needed
	// https://github.com/Azure/kperf/pull/198#discussion_r2252571111
	items *list.List
}

// InitCache creates a new empty cache
func InitCache() *Cache {
	return &Cache{
		items: list.New(),
	}
}

// Pop removes and returns the first item from the cache.
// Returns empty string and false if cache is empty.
func (c *Cache) Pop() (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.items.Len() == 0 {
		return "", false
	}

	// Remove from front (FIFO)
	front := c.items.Front()
	name := front.Value.(string)
	c.items.Remove(front)
	return name, true
}

// Push adds an item to the cache.
func (c *Cache) Push(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Add new item to back
	c.items.PushBack(name)
}

// Len returns the number of items in the cache.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.items.Len()
}
