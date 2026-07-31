package dagreach

// A counter that remembers the order keys were first seen.
//
// Text output follows declaration order, so a plain map would make the same
// graph render differently between runs. JSON output sorts its keys anyway.

type Counter struct {
	keys   []string
	counts map[string]int
}

func NewCounter() *Counter { return &Counter{counts: map[string]int{}} }

func (c *Counter) Add(key string) {
	if _, present := c.counts[key]; !present {
		c.keys = append(c.keys, key)
	}
	c.counts[key]++
}

func (c *Counter) Keys() []string { return c.keys }

func (c *Counter) Get(key string) int { return c.counts[key] }

func (c *Counter) Len() int { return len(c.keys) }

func (c *Counter) Map() map[string]int {
	copied := map[string]int{}
	for key, value := range c.counts {
		copied[key] = value
	}
	return copied
}
