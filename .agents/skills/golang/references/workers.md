# Workers e graceful shutdown

```go
workers := 8
var wg sync.WaitGroup
for i := 0; i < workers; i++ {
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case job, ok := <-jobs:
				if !ok { return }
				if err := process(ctx, job); err != nil {
					log.ErrorContext(ctx, "processar job", "error", err)
				}
			case <-ctx.Done(): return
			}
		}
	}()
}
wg.Wait()
```
