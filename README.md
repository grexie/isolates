# V8 Bindings for Go

## Usage

To use import v8:

```
import (
  v8 "github.com/behrsin/go-v8"
)
```

See the v8_isolate_test.go file for a basic example of how to setup a `v8.Isolate`, `v8.Context`, create values and run JavaScript code. For more detailed API information see [GoDoc](https://godoc.org/github.com/behrsin/go-v8).


## Copyright

Substantially based on the great work by Augusto Roman ([@augustoroman](https://github.com/augustoroman)):

[github.com/augustoroman/v8](https://github.com/augustoroman/v8)

I've added native promises, JSON stringify / parse, value caching, weak callbacks, function templates and constructors, v8
Inspector and a terminal-based allocation tracer api for debugging.

Thanks be to God for the help He has given me in writing this.

## Bugs

Please open an issue to report a bug. Before opening a new issue please see if there are already issues matching your
case.

## Installation

For now, please follow his instructions for installation of the v8 libraries and headers:

[https://github.com/augustoroman/v8](github.com/augustoroman/v8)

There's a script included `install-v8.sh` that can be used to install the version of libraries this library is developed
against for both ARMv6 and AMD64 (linux and macOS):

```
./path/to/behrsin/go-v8/install-v8.sh
```

This will download the binaries and install them into the go-v8 folder under `libv8/` and `include/`.

## Debug

There is a heap tracing tool which monitors active V8 objects referenced by Go. To use it look at `StartTracer` and `EnableAllocationStackTraces`. You can mount `TracerHandler()` using a `net/http.(*ServeMux)` to enable the stack traces over HTTP or you can use `v8.DumpTracer` to writer the trace output to a log file or `os.Stdout` periodically.
