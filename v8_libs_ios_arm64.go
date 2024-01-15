//go:build arm64 && ios

package isolates

/*
#include "v8_c_bridge.h"
#cgo CXXFLAGS: -I/usr/local/include/v8 -std=c++17
#cgo ios LDFLAGS: -L/usr/local/lib/v8/arm64/ios/release -pthread -lv8_monolith -lstdc++
*/
import "C"
