package isolates

//#include "v8_c_bridge.h"
//#cgo CXXFLAGS: -I/usr/local/include/v8 -std=c++17
import "C"

import (
	"context"
	"errors"
	"fmt"
	"log"
	"reflect"
	"runtime"
	"sync"
	"unsafe"

	refutils "github.com/grexie/refutils"
)

const contextKey = "github.com/grexie/isolates"

type ExecutionContext struct {
	refutils.RefHolder
	ctx            context.Context
	mutex          sync.Mutex
	locked         bool
	isolate        *Isolate
	context        *Context
	entrantMutex   sync.Mutex
	enterCallbacks []func()
	exitCallbacks  []func()
	enterSnapshot  HeapStatistics
}

func newIsolateContext(isolate *Isolate) *ExecutionContext {
	if isolate != nil && isolate.executionContext != nil {
		return For(isolate.executionContext)
	}
	context := &ExecutionContext{isolate: isolate, enterCallbacks: []func(){}, exitCallbacks: []func(){}}
	context.ref()
	return context
}

func WithContext(ctx context.Context) context.Context {
	executionContext := newIsolateContext(nil)
	ctx = context.WithValue(ctx, contextKey, executionContext)
	executionContext.ctx = ctx
	return ctx
}

func withIsolateContext(ctx context.Context, isolate *Isolate) context.Context {
	executionContext := newIsolateContext(isolate)
	ctx = context.WithValue(ctx, contextKey, executionContext)
	executionContext.ctx = ctx
	return ctx
}

func For(ctx context.Context) *ExecutionContext {
	return ctx.Value(contextKey).(*ExecutionContext)
}

func (ec *ExecutionContext) ref() refutils.ID {

	return executionContextRefs.Ref(ec)
}

func (ec *ExecutionContext) unref() {
	executionContextRefs.Unref(ec)
}

func (ec *ExecutionContext) Isolate() *Isolate {
	return ec.isolate
}

func (ec *ExecutionContext) Context() *Context {
	return ec.context
}

func (ec *ExecutionContext) SetContext(c *Context) {
	ec.context = c
}

func (ec *ExecutionContext) AddExecutionEnterCallback(callback func()) {
	ec.enterCallbacks = append(ec.enterCallbacks, callback)
}

func (ec *ExecutionContext) AddExecutionExitCallback(callback func()) {
	ec.exitCallbacks = append(ec.exitCallbacks, callback)
}

func (ec *ExecutionContext) enter() {
	for _, callback := range ec.enterCallbacks {
		callback()
	}
}

func (ec *ExecutionContext) exit() {
	for _, callback := range ec.exitCallbacks {
		callback()
	}
}

type callbackResult struct {
	value interface{}
	err   error
}
type callbackInfo struct {
	fn     func(context.Context) (interface{}, error)
	result chan callbackResult
}

type v8IsolateInitializer struct {
	fn     func() *Isolate
	result chan (*Isolate)
}

var v8IsolateInitializers chan v8IsolateInitializer = make(chan v8IsolateInitializer)

type Isolate struct {
	refutils.RefHolder

	pointer          C.IsolatePtr
	contexts         *refutils.RefMap
	mutex            sync.Mutex
	running          bool
	data             map[string]interface{}
	shutdownHooks    []interface{}
	lockerStackTrace []byte

	executionContext context.Context
	callbacks        chan callbackInfo
	close            chan bool
}

type Snapshot struct {
	data C.StartupData
}

type HeapStatistics struct {
	TotalHeapSize           uint64
	TotalHeapSizeExecutable uint64
	TotalPhysicalSize       uint64
	TotalAvailableSize      uint64
	UsedHeapSize            uint64
	HeapSizeLimit           uint64
	MallocedMemory          uint64
	PeakMallocedMemory      uint64
	DoesZapGarbage          bool
}

var isolateRefs = refutils.NewWeakRefMap("i")
var executionContextRefs = refutils.NewWeakRefMap("ec")

func NewIsolate() *Isolate {
	Initialize()

	callback := func() *Isolate {
		isolate := &Isolate{
			pointer:       C.v8_Isolate_New(C.StartupData{data: nil, length: 0}),
			contexts:      refutils.NewWeakRefMap("c"),
			running:       true,
			data:          map[string]interface{}{},
			shutdownHooks: []interface{}{},
			callbacks:     make(chan callbackInfo),
			close:         make(chan bool),
		}

		isolate.executionContext = withIsolateContext(context.Background(), isolate)

		isolate.ref()
		runtime.SetFinalizer(isolate, (*Isolate).release)

		tracer.Add(isolate)

		go isolate.processCallbacks()

		return isolate
	}

	ch := make(chan *Isolate)
	v8IsolateInitializers <- v8IsolateInitializer{callback, ch}
	return <-ch
}

func NewIsolateWithSnapshot(snapshot *Snapshot) *Isolate {
	Initialize()

	isolate := &Isolate{
		pointer:       C.v8_Isolate_New(snapshot.data),
		contexts:      refutils.NewWeakRefMap("c"),
		running:       true,
		data:          map[string]interface{}{},
		shutdownHooks: []interface{}{},
		callbacks:     make(chan callbackInfo),
		close:         make(chan bool),
	}
	isolate.executionContext = withIsolateContext(context.Background(), isolate)

	isolate.ref()
	runtime.SetFinalizer(isolate, (*Isolate).release)

	tracer.Add(isolate)

	go isolate.processCallbacks()

	return isolate
}

func (i *Isolate) GetExecutionContext() *ExecutionContext {
	return For(i.executionContext)
}

func (i *Isolate) ref() refutils.ID {
	return isolateRefs.Ref(i)
}

func (i *Isolate) unref() {
	isolateRefs.Unref(i)
}

func (i *Isolate) processCallbacks() {
	ctx := withIsolateContext(context.Background(), i)

	i.AddShutdownHook(ctx, func(i *Isolate) {
		i.close <- true
	})

	for true {
		select {
		case callback := <-i.callbacks:
			value, err := callback.fn(i.executionContext)
			if value == nil {
				callback.result <- callbackResult{nil, err}
			} else {
				callback.result <- callbackResult{value, err}
			}
		case <-i.close:
			return
		}
	}
}

func (i *Isolate) Sync(ctx context.Context, fn func(context.Context) (interface{}, error)) (interface{}, error) {
	executionContext := For(ctx)

	if locked := executionContext.entrantMutex.TryLock(); locked {
		executionContext.enter()
		defer executionContext.exit()
		defer executionContext.entrantMutex.Unlock()
	}

	if executionContext.isolate == i {
		return fn(i.executionContext)
	}

	ch := make(chan callbackResult)

	if i.pointer == nil {
		return nil, fmt.Errorf("isolate terminated")
	}

	i.callbacks <- callbackInfo{fn, ch}

	result := <-ch

	return result.value, result.err
}

func (i *Isolate) IsRunning(ctx context.Context) (bool, error) {
	return i.running, nil
}

func (i *Isolate) AddShutdownHook(ctx context.Context, shutdownHook interface{}) {
	i.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		i.shutdownHooks = append(i.shutdownHooks, shutdownHook)
		return nil, nil
	})
}

func (i *Isolate) GetData(key string) interface{} {
	return i.data[key]
}

func (i *Isolate) SetData(key string, value interface{}) {
	i.data[key] = value
}

func (i *Isolate) RequestGarbageCollectionForTesting(ctx context.Context) {
	C.v8_Isolate_RequestGarbageCollectionForTesting(i.pointer)
}

func (i *Isolate) Enter(ctx context.Context) {
	C.v8_Isolate_Enter(i.pointer)
}

func (i *Isolate) Exit(ctx context.Context) {
	C.v8_Isolate_Exit(i.pointer)
}

func (i *Isolate) PerformMicrotaskCheckpointSync(ctx context.Context) error {
	_, err := i.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		C.v8_Isolate_PerformMicrotaskCheckpoint(i.pointer)
		return nil, nil
	})
	return err
}

func (i *Isolate) PerformMicrotaskCheckpointInBackground() {
	go func() {
		ctx := withIsolateContext(context.Background(), i)
		if err := i.PerformMicrotaskCheckpointSync(ctx); err != nil {
			fmt.Println(err)
		}
	}()
}

func (i *Isolate) EnqueueMicrotaskWithValue(ctx context.Context, fn *Value) error {
	_, err := i.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		fn.context.ref()
		defer fn.context.unref()

		fn.Ref()
		defer fn.Unref()

		C.v8_Isolate_EnqueueMicrotask(i.pointer, fn.context.pointer, fn.pointer)
		return nil, nil
	})
	return err
}

func (i *Isolate) Background(ctx context.Context, callback func(ctx context.Context)) {
	go func() {
		defer func() {
			if p := recover(); p != nil {
				err := fmt.Errorf("recover panic:")
				log.Println(err)
				return
			}
		}()

		con := For(ctx).Context()
		ctx = withIsolateContext(context.Background(), i)
		For(ctx).SetContext(con)

		callback(ctx)
	}()
}

func (i *Isolate) Terminate() {
	ctx := withIsolateContext(context.Background(), i)
	i.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		log.Println("TERMINATING ISOLATE")
		vi := reflect.ValueOf(i)
		for _, shutdownHook := range i.shutdownHooks {
			reflect.ValueOf(shutdownHook).Call([]reflect.Value{vi})
		}
		i.shutdownHooks = nil

		isolateRefs.Release(i)
		C.v8_Isolate_Terminate(i.pointer)
		i.running = false

		contexts := i.contexts.Refs()
		for _, c := range contexts {
			context := c.(*Context)
			if context.pointer != nil {
				C.v8_Context_Release(context.pointer)
				context.pointer = nil
			}
		}

		C.v8_Isolate_Release(i.pointer)
		i.pointer = nil

		for _, context := range i.contexts.Refs() {
			context.(*Context).release()
		}

		tracer.Remove(i)
		isolateRefs.Release(i)

		i.data = nil

		return nil, nil
	})
}

func (i *Isolate) SendLowMemoryNotification(ctx context.Context) {
	C.v8_Isolate_LowMemoryNotification(i.pointer)
}

func (i *Isolate) GetHeapStatistics(ctx context.Context) (HeapStatistics, error) {
	hs := C.v8_Isolate_GetHeapStatistics(i.pointer)

	return HeapStatistics{
		TotalHeapSize:           uint64(hs.totalHeapSize),
		TotalHeapSizeExecutable: uint64(hs.totalHeapSizeExecutable),
		TotalPhysicalSize:       uint64(hs.totalPhysicalSize),
		TotalAvailableSize:      uint64(hs.totalAvailableSize),
		UsedHeapSize:            uint64(hs.usedHeapSize),
		HeapSizeLimit:           uint64(hs.heapSizeLimit),
		MallocedMemory:          uint64(hs.mallocedMemory),
		PeakMallocedMemory:      uint64(hs.peakMallocedMemory),
		DoesZapGarbage:          hs.doesZapGarbage == 1,
	}, nil
}

func (i *Isolate) newError(err C.Error) error {
	if err.data == nil {
		return nil
	}
	out := errors.New(C.GoStringN(err.data, err.length))
	C.free(unsafe.Pointer(err.data))
	return out
}

func (i *Isolate) release() {
	i.Terminate()
}

// func newSnapshot(data C.StartupData) *Snapshot {
// 	s := &Snapshot{data}
// 	runtime.SetFinalizer(s, (*Snapshot).release)
// 	return s
// }

// func CreateSnapshot(code string) *Snapshot {
// 	initOnce.Do(func() {
// 		C.v8_Initialize()
// 	})

// 	pcode := C.CString(code)
// 	defer C.free(unsafe.Pointer(pcode))

// 	return newSnapshot(C.v8_CreateSnapshotDataBlob(pcode))
// }

func (s *Snapshot) release() {
	if s.data.data != nil {
		C.free(unsafe.Pointer(s.data.data))
	}
	s.data.data = nil
	s.data.length = 0
	runtime.SetFinalizer(s, nil)
}

func (s *Snapshot) Export() []byte {
	return []byte(C.GoStringN(s.data.data, s.data.length))
}

// func ImportSnapshot(data []byte) *Snapshot {
// 	pdata := C.String{
// 		data:   (*C.char)(C.malloc(C.size_t(len(data)))),
// 		length: C.int(len(data)),
// 	}
// 	C.memcpy(unsafe.Pointer(pdata.data), unsafe.Pointer(&data[0]), C.size_t(len(data)))
// 	return newSnapshot(pdata)
// }
