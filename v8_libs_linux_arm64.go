//go:build arm64 && linux

package isolates

//#include "v8_c_bridge.h"
//#cgo CXXFLAGS: -I/usr/local/include/v8 -std=c++17
//#cgo linux LDFLAGS: -L/usr/local/lib/v8/arm64/linux -pthread -lv8_monolith
import "C"
