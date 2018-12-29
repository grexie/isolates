package v8

import (
	"fmt"
	"io"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type tracerObject interface {
	reference
	tracerString() string
}

type tracer interface {
	AddContext(context *Context)
	RemoveContext(context *Context)
	AddValue(value *Value)
	RemoveValue(value *Value)
	AddReferenceMap(name string, referenceMap *referenceMap)
	Dump(w io.Writer, allocations bool)
	Lock()
	Unlock()
}

type TracerType uint8

const (
	SimpleTracer TracerType = iota
)

func (i *Isolate) StartTracer(t TracerType) {
	switch t {
	case SimpleTracer:
		i.tracer = newSimpleTracer(i)
	}

	i.tracer.AddReferenceMap("isolates", isolates)
	i.tracer.AddReferenceMap("contexts", i.contexts)
}

func (i *Isolate) StopTracer(t TracerType) {
	i.tracer = &nullTracer{}
}

func (i *Isolate) DumpTracer(w io.Writer, allocations bool) {
	go i.tracer.Dump(w, allocations)
}

type nullTracer struct {
}

func (t *nullTracer) AddContext(context *Context) {

}

func (t *nullTracer) RemoveContext(context *Context) {

}

func (t *nullTracer) AddValue(value *Value) {

}

func (t *nullTracer) RemoveValue(value *Value) {

}

func (t *nullTracer) AddReferenceMap(name string, referenceMap *referenceMap) {

}

func (t *nullTracer) Dump(w io.Writer, allocations bool) {

}

func (t *nullTracer) Lock() {

}

func (t *nullTracer) Unlock() {

}

type simpleTracer struct {
	isolate        *Isolate
	values         *referenceMap
	referenceMaps  map[string]*referenceMap
	mutex          *sync.Mutex
	acquiredLock   uint32
	acquiringMutex *sync.Mutex
}

func newSimpleTracer(i *Isolate) *simpleTracer {
	t := &simpleTracer{
		isolate:        i,
		values:         newWeakReferenceMap("tracer", reflect.TypeOf(&Value{})),
		referenceMaps:  map[string]*referenceMap{},
		mutex:          &sync.Mutex{},
		acquiringMutex: &sync.Mutex{},
	}
	return t
}

func (t *simpleTracer) AddContext(context *Context) {
	t.AddReferenceMap("functionInfos", context.functions)
	t.AddReferenceMap("accessorInfos", context.accessors)
	t.AddReferenceMap("valueRefs", context.values)
	t.AddReferenceMap("refs", context.refs)
}

func (t *simpleTracer) RemoveContext(context *Context) {

}

func (t *simpleTracer) AddValue(value *Value) {
	t.values.Ref(value)
}

func (t *simpleTracer) RemoveValue(value *Value) {
	t.values.Unref(value)
}

func (t *simpleTracer) AddReferenceMap(name string, referenceMap *referenceMap) {
	t.referenceMaps[name] = referenceMap
}

func sortedMapStringString(m map[string]string, f func(k string, v string)) {
	var keys []string
	for k, _ := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		f(k, m[k])
	}
}

func (t *simpleTracer) Dump(w io.Writer, allocations bool) {
	runtime.GC()
	t.isolate.RequestGarbageCollectionForTesting()

	t.mutex.Lock()
	defer t.mutex.Unlock()

	values := t.values.References()
	valuesCreated := []reference{}
	for _, v := range values {
		if v.(*Value).created {
			valuesCreated = append(valuesCreated, v)
		}
	}

	stats := map[string]string{}

	fmt.Fprintf(w, "%s\n", strings.Repeat("=", 80))
	fmt.Fprintf(w, "V8 Golang Tracer Dump\n%s\n", time.Now())
	fmt.Fprintf(w, "%s\n", strings.Repeat("-", 80))
	stats["values"] = fmt.Sprintf("%d (of which %d are owned by go)", len(values), len(valuesCreated))

	for name, referenceMap := range t.referenceMaps {
		stats[name] = fmt.Sprintf("%d", referenceMap.Length())
	}

	sortedMapStringString(stats, func(name, value string) {
		fmt.Fprintf(w, "%s: %s\n", name, value)
	})

	stats = map[string]string{}
	fmt.Fprintf(w, "%s\n", strings.Repeat("-", 80))
	fmt.Fprintf(w, "V8 Isolate Heap Statistics:\n\n")

	hs := t.isolate.GetHeapStatistics()
	stats["total heap size"] = fmt.Sprintf("%d", hs.TotalHeapSize)
	stats["total heap size executable"] = fmt.Sprintf("%d", hs.TotalHeapSizeExecutable)
	stats["total physical size"] = fmt.Sprintf("%d", hs.TotalPhysicalSize)
	stats["total available size"] = fmt.Sprintf("%d", hs.TotalAvailableSize)
	stats["used heap size"] = fmt.Sprintf("%d", hs.UsedHeapSize)
	stats["heap size limit"] = fmt.Sprintf("%d", hs.HeapSizeLimit)
	stats["malloced memory"] = fmt.Sprintf("%d", hs.MallocedMemory)
	stats["peak malloced memory"] = fmt.Sprintf("%d", hs.PeakMallocedMemory)
	stats["does zap garbage"] = fmt.Sprintf("%t", hs.DoesZapGarbage)

	sortedMapStringString(stats, func(name, value string) {
		fmt.Fprintf(w, "%s: %s\n", name, value)
	})

	if allocations {
		fmt.Fprintf(w, "%s\n", strings.Repeat("-", 80))

		for i, ref := range values {
			object := ref.(*Value)

			fmt.Fprintf(w, "  0x%08x (%s): %s\n", i, "v8.Value", object.tracerString())
		}
	}

	fmt.Fprintf(w, "%s\n", strings.Repeat("-", 80))
}

func MyCaller() string {

	// we get the callers as uintptrs - but we just need 1
	fpcs := make([]uintptr, 1)

	// skip 3 levels to get to the caller of whoever called Caller()
	n := runtime.Callers(3, fpcs)
	if n == 0 {
		return "n/a" // proper error her would be better
	}

	// get the info of the actual function that's in the pointer
	fun := runtime.FuncForPC(fpcs[0] - 1)
	if fun == nil {
		return "n/a"
	}

	// return its name
	return fun.Name()
}

func (t *simpleTracer) Lock() {
	t.acquiringMutex.Lock()
	defer t.acquiringMutex.Unlock()

	if t.acquiredLock == 0 {
		t.mutex.Lock()
	}
	t.acquiredLock++
}

func (t *simpleTracer) Unlock() {
	t.acquiringMutex.Lock()
	defer t.acquiringMutex.Unlock()

	t.acquiredLock--
	if t.acquiredLock == 0 {
		t.mutex.Unlock()
	}
}
