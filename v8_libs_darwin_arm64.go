//go:build arm64 && darwin && !ios && !v8_debug

package isolates

/*
#include "v8_c_bridge.h"
#cgo CXXFLAGS: -I/usr/local/include/v8 -std=c++17 -Wimplicit-function-declaration
#cgo darwin LDFLAGS: -L/usr/local/lib/v8/arm64/macos/release -pthread -lv8_monolith -lstdc++
*/
import "C"
