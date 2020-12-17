package v8

// #include "v8_c_bridge.h"
// #cgo CXXFLAGS: -I${SRCDIR} -I${SRCDIR}/include -g3 -fno-rtti -fpic -std=c++14
// #cgo LDFLAGS: -pthread -L${SRCDIR}/libv8 -lv8_base -lv8_init -lv8_initializers -lv8_libbase -lv8_libplatform -lv8_libsampler -lv8_nosnapshot
import "C"

import (
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"unsafe"

	refutils "github.com/behrsin/go-refutils"
)

type Isolate struct {
	refutils.RefHolder

	pointer       C.IsolatePtr
	contexts      *refutils.RefMap
	mutex         refutils.RefMutex
	running       bool
	data          map[string]interface{}
	shutdownHooks []interface{}
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

var isolates = refutils.NewWeakRefMap("i")

func NewIsolate() *Isolate {
	Initialize()

	isolate := &Isolate{
		pointer:       C.v8_Isolate_New(C.StartupData{data: nil, length: 0}),
		contexts:      refutils.NewWeakRefMap("c"),
		running:       true,
		data:          map[string]interface{}{},
		shutdownHooks: []interface{}{},
	}
	isolate.ref()
	runtime.SetFinalizer(isolate, (*Isolate).release)

	tracer.Add(isolate)

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
	}
	isolate.ref()
	runtime.SetFinalizer(isolate, (*Isolate).release)

	tracer.Add(isolate)

	return isolate
}

func (i *Isolate) ref() refutils.ID {
	return isolates.Ref(i)
}

func (i *Isolate) unref() {
	isolates.Unref(i)
}

func (i *Isolate) lock() error {
	i.mutex.RefLock()
	if !i.running {
		defer i.mutex.RefUnlock()
		return fmt.Errorf("isolate terminated")
	}
	return nil
}

func (i *Isolate) unlock() {
	i.mutex.RefUnlock()
}

func (i *Isolate) IsRunning() bool {
	i.mutex.RefLock()
	defer i.mutex.RefUnlock()

	return i.running
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

func (i *Isolate) RequestGarbageCollectionForTesting() {
	if err := i.lock(); err != nil {
		return
	} else {
		defer i.unlock()
	}

	C.v8_Isolate_RequestGarbageCollectionForTesting(i.pointer)
}

func (i *Isolate) Terminate() {
	runtime.SetFinalizer(i, nil)
	i.mutex.Lock()
	if !i.running {
		i.mutex.Unlock()
		return
	}

	isolates.Release(i)
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
	isolates.Release(i)

	vi := reflect.ValueOf(i)
	for _, shutdownHook := range i.shutdownHooks {
		reflect.ValueOf(shutdownHook).Call([]reflect.Value{vi})
	}
	i.shutdownHooks = nil

	i.data = nil
}

func (i *Isolate) SendLowMemoryNotification() {
	if err := i.lock(); err != nil {
		return
	} else {
		defer i.unlock()
	}

	C.v8_Isolate_LowMemoryNotification(i.pointer)
}

func (i *Isolate) GetHeapStatistics() (HeapStatistics, error) {
	if err := i.lock(); err != nil {
		return HeapStatistics{}, err
	} else {
		defer i.unlock()
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
