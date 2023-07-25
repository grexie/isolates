//go:build amd64 && ios

package isolates

/*
#include "v8_c_bridge.h"
#cgo CXXFLAGS: -I/usr/local/include/v8 -std=c++17
#cgo ios LDFLAGS: -L/usr/local/lib/v8/x64/ios -pthread -lv8_monolith -lstdc++
*/
import "C"
