//go:build v8_tracer

package isolates

//#include "v8_c_bridge.h"
//#cgo CXXFLAGS: -I/usr/local/include/v8 -std=c++17

import "C"

import (
	"fmt"
	"runtime"
	"sync"
)

type allocation struct {
	RefCount   int
	StackTrace []byte
}

type _tracer struct {
	mutex    sync.Mutex
	retained map[any]allocation
	released map[any]allocation
}

var tracer = newTracer()

func newTracer() *_tracer {
	return &_tracer{
		retained: map[any]allocation{},
		released: map[any]allocation{},
	}
}

func (t *_tracer) Retain(object any) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if a, ok := t.released[object]; ok {
		fmt.Println("\n\n*******************")
		fmt.Println("*** UNDERRETAIN ***")
		fmt.Println("*******************\n\n")
		fmt.Println("First retained here:\n", string(a.StackTrace))
		fmt.Println("*******************\n\n")
		panic("under retain")
	}

	if a, ok := t.retained[object]; ok {
		a.RefCount++
	} else {
		stack := make([]byte, 40*1024)
		n := runtime.Stack(stack, false)
		t.retained[object] = allocation{RefCount: 1, StackTrace: stack[:n]}
	}
}

func (t *_tracer) Release(object any) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if a, ok := t.released[object]; ok {
		fmt.Println("\n\n*********************")
		fmt.Println("*** OVERRELEASE ***")
		fmt.Println("*******************\n\n")
		fmt.Println("First retained here:\n", string(a.StackTrace))
		fmt.Println("*******************\n\n")
	}

	if allocation, ok := t.retained[object]; ok {
		allocation.RefCount--
		if allocation.RefCount == 0 {
			delete(t.retained, object)
			t.released[object] = allocation
		}
	} else {
		fmt.Println("\n\n******************************")
		fmt.Println("*** RELEASE WITHOUT RETAIN ***")
		fmt.Println("******************************\n\n")
		panic("release without retain")
	}
}
