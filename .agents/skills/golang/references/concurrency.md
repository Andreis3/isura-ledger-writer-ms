# Estado e concorrência

```go
type Counter struct {
	mu    sync.Mutex
	value int
}

func (c *Counter) Increment() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value++
	return c.value
}
```
