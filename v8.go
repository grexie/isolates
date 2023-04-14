package isolates

// #include "v8_c_bridge.h"
// #cgo CXXFLAGS: -I/usr/local/include/v8 -g3 -fno-rtti -fpic -std=c++20
// #cgo LDFLAGS: -L/usr/local/lib/v8/arm64/macos -pthread -lv8_base_without_compiler -lv8_libbase -lv8_libplatform -lv8_snapshot -lv8_bigint -licui18n -licuuc -lv8_compiler -lv8_heap_base -lcppgc_base
// #cgo darwin,amd64 LDFLAGS: -L/usr/local/lib/v8/x64/macos -pthread -lv8_base_without_compiler -lv8_init -lv8_initializers -lv8_libbase -lv8_libplatform -lv8_snapshot
// #cgo linux,arm64 LDFLAGS: -L/usr/local/lib/v8/arm64/linux -pthread -lv8_base_without_compiler -lv8_init -lv8_initializers -lv8_libbase -lv8_libplatform -lv8_snapshot
// #cgo linux,amd64 LDFLAGS: -L/usr/local/lib/v8/x64/linux -pthread -lv8_base_without_compiler -lv8_init -lv8_initializers -lv8_libbase -lv8_libplatform -lv8_snapshot
import "C"

import (
	"fmt"
	"sync"
)

// Version exposes the compiled-in version of the linked V8 library.  This can
// be used to test for specific javascript functionality support (e.g. ES6
// destructuring isn't supported before major version 5.).
var Version = struct{ Major, Minor, Build, Patch int }{
	Major: int(C.version.major),
	Minor: int(C.version.minor),
	Build: int(C.version.build),
	Patch: int(C.version.patch),
}

// PromiseState defines the state of a promise: either pending, resolved, or
// rejected. Promises that are pending have no result value yet. A promise that
// is resolved has a result value, and a promise that is rejected has a result
// value that is usually the error.
type PromiseState uint8

const (
	PromiseStatePending PromiseState = iota
	PromiseStateResolved
	PromiseStateRejected
	kNumPromiseStates
)

var promiseStateStrings = [kNumPromiseStates]string{"Pending", "Resolved", "Rejected"}

func (s PromiseState) String() string {
	if s < 0 || s >= kNumPromiseStates {
		return fmt.Sprintf("InvalidPromiseState:%d", int(s))
	}
	return promiseStateStrings[s]
}

var initOnce sync.Once

func Initialize() {
	initOnce.Do(func() {
		C.v8_Initialize()
	})
}
