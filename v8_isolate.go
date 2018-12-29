package v8

// #include "v8_c_bridge.h"
// #cgo CXXFLAGS: -I${SRCDIR} -I${SRCDIR}/include -g3 -fno-rtti -fpic -std=c++11
// #cgo LDFLAGS: -pthread -L${SRCDIR}/libv8 -lv8_base -lv8_init -lv8_initializers -lv8_libbase -lv8_libplatform -lv8_libsampler -lv8_nosnapshot
import "C"

import (
	"errors"
	"reflect"
	"runtime"
	"sync"
	"unsafe"
)

type Isolate struct {
	referenceObject
	pointer  C.IsolatePtr
	contexts *referenceMap
	tracer   tracer
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

var initOnce sync.Once
var isolates = newReferenceMap("i", reflect.TypeOf(&Isolate{}))

func NewIsolate() *Isolate {
	initOnce.Do(func() {
		C.v8_Initialize()
	})

	isolate := &Isolate{
		pointer:  C.v8_Isolate_New(C.StartupData{data: nil, length: 0}),
		contexts: newReferenceMap("c", reflect.TypeOf(&Context{})),
		tracer:   &nullTracer{},
	}
	isolate.ref()
	runtime.SetFinalizer(isolate, (*Isolate).release)

	return isolate
}

func NewIsolateWithSnapshot(snapshot *Snapshot) *Isolate {
	initOnce.Do(func() {
		C.v8_Initialize()
	})

	isolate := &Isolate{
		pointer:  C.v8_Isolate_New(snapshot.data),
		contexts: newReferenceMap("c", reflect.TypeOf(&Context{})),
		tracer:   &nullTracer{},
	}
	isolate.ref()
	runtime.SetFinalizer(isolate, (*Isolate).release)

	return isolate
}

func (i *Isolate) ref() id {
	return isolates.Ref(i)
}

func (i *Isolate) unref() {
	isolates.Unref(i)
}

func (i *Isolate) RequestGarbageCollectionForTesting() {
	C.v8_Isolate_RequestGarbageCollectionForTesting(i.pointer)
}

func (i *Isolate) Terminate() {
	C.v8_Isolate_Terminate(i.pointer)
	i.release()
}

func (i *Isolate) SendLowMemoryNotification() {
	C.v8_Isolate_LowMemoryNotification(i.pointer)
}

func (i *Isolate) GetHeapStatistics() HeapStatistics {
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
	}
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
	C.v8_Isolate_Release(i.pointer)
	i.pointer = nil
	isolates.Release(i)
	runtime.SetFinalizer(i, nil)
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
