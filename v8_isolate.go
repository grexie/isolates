package isolates

// #include "v8_c_bridge.h"
import "C"

import (
	"context"
	"errors"
	"fmt"
	"time"

	"reflect"
	"runtime"
	"runtime/debug"
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
	entrantMutex   sync.Mutex
	enterCallbacks []func()
	exitCallbacks  []func()
	enterSnapshot  HeapStatistics
}

func newIsolateContext(isolate *Isolate) *ExecutionContext {
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

func FromContext(ctx context.Context) *ExecutionContext {
	return ctx.Value(contextKey).(*ExecutionContext)
}

func (ec *ExecutionContext) ref() refutils.ID {

	return executionContextRefs.Ref(ec)
}

func (ec *ExecutionContext) unref() {
	executionContextRefs.Unref(ec)
}

func (ec *ExecutionContext) GetIsolate() *Isolate {
	return ec.isolate
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
	value *Value
	err   error
}
type callbackInfo struct {
	fn     func(context.Context) (*Value, error)
	result chan callbackResult
}

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
	return FromContext(i.executionContext)
}

func (i *Isolate) ref() refutils.ID {
	return isolateRefs.Ref(i)
}

func (i *Isolate) unref() {
	isolateRefs.Unref(i)
}

func (i *Isolate) processCallbacks() {
	i.AddShutdownHook(func(i *Isolate) {
		i.close <- true
	})

	for true {
		select {
		case callback := <-i.callbacks:
			value, err := callback.fn(i.executionContext)
			callback.result <- callbackResult{value, err}
		case <-i.close:
			return
		}
	}
}

func (i *Isolate) Sync(ctx context.Context, fn func(context.Context) (*Value, error)) (*Value, error) {
	executionContext := FromContext(ctx)

	if locked := executionContext.entrantMutex.TryLock(); locked {
		executionContext.enter()
		defer executionContext.exit()
		defer executionContext.entrantMutex.Unlock()
	}

	if executionContext.isolate == i {
		return fn(i.executionContext)
	}

	ch := make(chan callbackResult)

	i.callbacks <- callbackInfo{fn, ch}

	result := <-ch

	return result.value, result.err
}

func (i *Isolate) lock(ctx context.Context) (bool, error) {
	context := FromContext(ctx)
	// fmt.Println("locked", context.locked)

	if context == nil {
		panic("no context found")
		return false, fmt.Errorf("context not found")
	}

	context.mutex.Lock()
	defer context.mutex.Unlock()

	if !context.locked {
		stack := debug.Stack()

		timer := time.AfterFunc(1*time.Second, func() {
			fmt.Println("\n\n\n\n----- LOCKED STACK TRACE\n\n\n")
			if i.lockerStackTrace != nil {
				fmt.Println(string(i.lockerStackTrace))
			} else {
				fmt.Println("locked but no stack trace available")
			}

			fmt.Println("\n\n\n\n----- LOCKING STACK TRACE\n\n\n")
			fmt.Println(string(stack))
			// os.Exit(1)
		})
		context.locked = true
		i.mutex.Lock()

		timer.Stop()

		i.lockerStackTrace = stack
		if !i.running {
			context.locked = false
			defer i.mutex.Unlock()
			return false, fmt.Errorf("isolate terminated")
		}
	} else {
		if !i.running {
			return false, fmt.Errorf("isolate terminated")
		} else {
			return false, nil
		}
	}

	return true, nil
}

func (i *Isolate) unlock(ctx context.Context) {
	context := FromContext(ctx)
	if context == nil {
		panic("no context found")
	}

	context.mutex.Lock()
	defer context.mutex.Unlock()

	if !context.locked {
		panic("not locked in this context")
	}

	i.mutex.Unlock()
	i.lockerStackTrace = nil
	context.locked = false

}

func (i *Isolate) IsRunning(ctx context.Context) (bool, error) {
	if locked, err := i.lock(ctx); err != nil {
		return false, err
	} else if locked {
		defer i.unlock(ctx)
	}

	return i.running, nil
}

func (i *Isolate) IsActive() bool {
	if locked := i.mutex.TryLock(); locked {
		defer i.mutex.Unlock()

		return true
	}

	return false
}

func (i *Isolate) AddShutdownHook(shutdownHook interface{}) {
	i.shutdownHooks = append(i.shutdownHooks, shutdownHook)
}

func (i *Isolate) GetData(key string) interface{} {
	return i.data[key]
}

func (i *Isolate) SetData(key string, value interface{}) {
	i.data[key] = value
}

func (i *Isolate) RequestGarbageCollectionForTesting(ctx context.Context) {
	if locked, err := i.lock(ctx); err != nil {
		return
	} else if locked {
		defer i.unlock(ctx)
	}

	C.v8_Isolate_RequestGarbageCollectionForTesting(i.pointer)
}

func (i *Isolate) Enter(ctx context.Context) {
	if locked, err := i.lock(ctx); err != nil {
		return
	} else if locked {
		defer i.unlock(ctx)
	}

	C.v8_Isolate_Enter(i.pointer)
}

func (i *Isolate) Exit(ctx context.Context) {
	if locked, err := i.lock(ctx); err != nil {
		return
	} else if locked {
		defer i.unlock(ctx)
	}

	C.v8_Isolate_Exit(i.pointer)
}

func (i *Isolate) RunMicrotasksSync(ctx context.Context) error {
	_, err := i.Sync(ctx, func(ctx context.Context) (*Value, error) {
		if locked, err := i.lock(ctx); err != nil {
			return nil, err
		} else if locked {
			defer i.unlock(ctx)
		}

		C.v8_Isolate_RunMicrotasks(i.pointer)
		return nil, nil
	})
	return err
}

func (i *Isolate) RunMicrotasksInBackground() {
	go func() {
		ctx := WithContext(context.Background())
		if err := i.RunMicrotasksSync(ctx); err != nil {
			fmt.Println(err)
		}
	}()
}

func (i *Isolate) EnqueueMicrotaskWithValue(ctx context.Context, fn *Value) error {
	_, err := i.Sync(ctx, func(ctx context.Context) (*Value, error) {
		if locked, err := i.lock(ctx); err != nil {
			return nil, err
		} else if locked {
			defer i.unlock(ctx)
		}

		fn.context.ref()
		defer fn.context.unref()

		fn.Ref()
		defer fn.Unref()

		C.v8_Isolate_EnqueueMicrotask(i.pointer, fn.context.pointer, fn.pointer)
		return nil, nil
	})
	return err
}

func (i *Isolate) Terminate() {
	// runtime.SetFinalizer(i, nil)
	i.mutex.Lock()
	if !i.running {
		i.mutex.Unlock()
		return
	}

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
	i.mutex.Unlock()

	for _, context := range i.contexts.Refs() {
		context.(*Context).release()
	}

	tracer.Remove(i)
	isolateRefs.Release(i)

	vi := reflect.ValueOf(i)
	for _, shutdownHook := range i.shutdownHooks {
		reflect.ValueOf(shutdownHook).Call([]reflect.Value{vi})
	}
	i.shutdownHooks = nil

	i.data = nil
}

func (i *Isolate) SendLowMemoryNotification(ctx context.Context) {
	if locked, err := i.lock(ctx); err != nil {
		return
	} else if locked {
		defer i.unlock(ctx)
	}

	C.v8_Isolate_LowMemoryNotification(i.pointer)
}

func (i *Isolate) GetHeapStatistics(ctx context.Context) (HeapStatistics, error) {
	if locked, err := i.lock(ctx); err != nil {
		return HeapStatistics{}, err
	} else if locked {
		defer i.unlock(ctx)
	}

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

func newSnapshot(data C.StartupData) *Snapshot {
	s := &Snapshot{data}
	runtime.SetFinalizer(s, (*Snapshot).release)
	return s
}

func CreateSnapshot(code string) *Snapshot {
	initOnce.Do(func() {
		C.v8_Initialize()
	})

	pcode := C.CString(code)
	defer C.free(unsafe.Pointer(pcode))

	return newSnapshot(C.v8_CreateSnapshotDataBlob(pcode))
}

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

func ImportSnapshot(data []byte) *Snapshot {
	pdata := C.String{
		data:   (*C.char)(C.malloc(C.size_t(len(data)))),
		length: C.int(len(data)),
	}
	C.memcpy(unsafe.Pointer(pdata.data), unsafe.Pointer(&data[0]), C.size_t(len(data)))
	return newSnapshot(pdata)
}
