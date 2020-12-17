package v8

// #include "v8_c_bridge.h"
// #cgo CXXFLAGS: -I${SRCDIR} -I${SRCDIR}/include -g3 -fno-rtti -fpic -std=c++14
// #cgo LDFLAGS: -pthread -L${SRCDIR}/libv8 -lv8_base -lv8_init -lv8_initializers -lv8_libbase -lv8_libplatform -lv8_libsampler -lv8_nosnapshot
import "C"

import (
	"runtime"
	"unsafe"
)

var nextInspectorID = 0
var inspectors = map[int]*Inspector{}

type InspectorCallbacks interface {
	V8InspectorSendResponse(callId int, message string)
	V8InspectorSendNotification(message string)
	V8InspectorFlushProtocolNotifications()
}

type Inspector struct {
	ptr       C.InspectorPtr
	id        int
	callbacks InspectorCallbacks
}

func (i *Isolate) NewInspector(callbacks InspectorCallbacks) *Inspector {
	inspectorID := nextInspectorID
	nextInspectorID++
	inspector := &Inspector{C.v8_Inspector_New(i.pointer, C.int(inspectorID)), inspectorID, callbacks}
	inspectors[inspectorID] = inspector
	runtime.SetFinalizer(inspector, (*Inspector).Release)
	return inspector
}

func (i *Inspector) AddContext(context *Context, name string) {
	pname := C.CString(name)
	defer C.free(unsafe.Pointer(pname))

	context.ref()
	C.v8_Inspector_AddContext(i.ptr, context.pointer, pname)
}

func (i *Inspector) RemoveContext(context *Context) {
	C.v8_Inspector_RemoveContext(i.ptr, context.pointer)
	context.unref()
}

func (i *Inspector) DispatchMessage(message string) {
	messageCStr := C.CString(message)
	C.v8_Inspector_DispatchMessage(i.ptr, messageCStr)
	C.free(unsafe.Pointer(messageCStr))
}

func (i *Inspector) Release() {
	// TODO remove all contexts that have been referenced in AddContext, RemoveContext
	if i.ptr != nil {
		C.v8_Inspector_Release(i.ptr)
	}
	i.ptr = nil

	delete(inspectors, i.id)
}

//export inspectorSendResponse
func inspectorSendResponse(inspectorID C.int, callID C.int, message C.String) {
	m := C.GoStringN(message.data, message.length)
	go inspectors[int(inspectorID)].callbacks.V8InspectorSendResponse(int(callID), m)
}

//export inspectorSendNotification
func inspectorSendNotification(inspectorID C.int, message C.String) {
	m := C.GoStringN(message.data, message.length)
	go inspectors[int(inspectorID)].callbacks.V8InspectorSendNotification(m)
}

//export inspectorFlushProtocolNotifications
func inspectorFlushProtocolNotifications(inspectorID C.int) {
	go inspectors[int(inspectorID)].callbacks.V8InspectorFlushProtocolNotifications()
}
