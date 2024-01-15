package isolates

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	StartTracer(SimpleTracer)
	Initialize()
	os.Exit(m.Run())
}

func DumpTracerForBenchmark(b *testing.B) {
	ctx := v.context.isolate.GetExecutionContext().ctx
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	DumpTracer(ctx, w, false)
	w.Flush()
	b.Logf("\n%s", string(buf.Bytes()))
}

func TestIsolateCreate(t *testing.T) {
	ctx := v.context.isolate.GetExecutionContext().ctx
	i := NewIsolate()
	if c, err := i.NewContext(ctx); err != nil {
		t.Error(err)
	} else if value, err := c.Create(ctx, 20); err != nil {
		t.Error(err)
	} else if fn, err := c.Run(ctx, `
		(() => {
			const fib = (n) => {
				if (n < 2) {
					return n;
				}
				return fib(n - 1) + fib(n - 2);
			}
			return fib;
		})()
	`, "index.js"); err != nil {
		t.Error(err)
	} else if result, err := fn.Call(ctx, nil, value); err != nil {
		t.Error(err)
	} else if n, err := result.Int64(ctx); err != nil {
		t.Error(err)
	} else if n != 6765 {
		t.Errorf("invalid result: %v", result)
		return
	}
	i.Terminate()
}

func BenchmarkIsolateCreate(b *testing.B) {
	runtime.GC()
	finished := make(chan bool)

	for n := 0; n < b.N; n++ {
		i := NewIsolate()

		go func(i *Isolate) {
			ctx := WithContext(context.Background())

			done := false

			go func() {
				time.Sleep(1 * time.Second)
				if !done {
					DumpTracerForBenchmark(b)
					b.Error("isolate is locked")
				}
			}()

			if c, err := i.NewContext(ctx); err != nil {
				b.Error(err)
			} else if value, err := c.Create(ctx, 20); err != nil {
				b.Error(err)
			} else if fn, err := c.Run(ctx, `
				(() => {
					const fib = (n) => {
						if (n < 2) {
							return n;
						}
						return fib(n - 1) + fib(n - 2);
					}
					return fib;
				})()
			`, "index.js"); err != nil {
				b.Error(err)
			} else if result, err := fn.Call(ctx, nil, value); err != nil {
				b.Error(err)
			} else if n, err := result.Int64(ctx); err != nil {
				b.Error(err)
			} else if n != 6765 {
				b.Errorf("invalid result: %v", result)
				return
			}

			done = true
			finished <- true
		}(i)
	}

	i := 0
	for {
		select {
		case <-time.After(20 * time.Second):
			DumpTracerForBenchmark(b)
			b.Error("v8 locked")
		case <-finished:
			i++
			if i == b.N {
				goto FINISHED
			}
		}
	}
FINISHED:
	close(finished)

	for _, isolate := range isolateRefs.Refs() {
		isolate.(*Isolate).Terminate()
	}

	runtime.GC()

	if isolateRefs.Length() != 0 {
		b.Errorf("%d isolates remaining after garbage collection", isolateRefs.Length())
	}

}
