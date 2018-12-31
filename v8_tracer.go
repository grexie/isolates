package v8

import (
	"fmt"
	"io"
	"log"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	refutils "github.com/behrsin/go-refutils"
)

type itracer interface {
	Start()
	Stop()
	EnableAllocationStackTraces()
	DisableAllocationStackTraces()
	Add(value refutils.Ref)
	Remove(value refutils.Ref)
	AddRefMap(name string, referenceMap *refutils.RefMap)
	RemoveRefMap(name string, referenceMap *refutils.RefMap)
	Dump(w io.Writer, allocations bool)
}

type TracerType uint8

const (
	SimpleTracer TracerType = iota
)

var tracer = itracer(&nullTracer{})

func EnableAllocationStackTraces() {
	tracer.EnableAllocationStackTraces()
}

func DisableAllocationStackTraces() {
	tracer.DisableAllocationStackTraces()
}

func StartTracer(t TracerType) {
	tracer.Stop()

	switch t {
	case SimpleTracer:
		tracer = newSimpleTracer()
	}

	tracer.Start()
}

func StopTracer(t TracerType) {
	tracer.Stop()
	tracer = &nullTracer{}
}

func DumpTracer(w io.Writer, allocations bool) {
	tracer.Dump(w, allocations)
}

type nullTracer struct{}

func (t *nullTracer) Start()                                            {}
func (t *nullTracer) Stop()                                             {}
func (t *nullTracer) EnableAllocationStackTraces()                      {}
func (t *nullTracer) DisableAllocationStackTraces()                     {}
func (t *nullTracer) Add(value refutils.Ref)                            {}
func (t *nullTracer) Remove(value refutils.Ref)                         {}
func (t *nullTracer) AddRefMap(name string, refMap *refutils.RefMap)    {}
func (t *nullTracer) RemoveRefMap(name string, refMap *refutils.RefMap) {}
func (t *nullTracer) Dump(w io.Writer, allocations bool)                {}

type simpleTracerAddMessage struct {
	Ref        refutils.Ref
	StackTrace []byte
}

type simpleTracerRefMapMessage struct {
	Name   string
	RefMap *refutils.RefMap
}

type simpleTracerMessage struct {
	Add          *simpleTracerAddMessage
	Remove       refutils.Ref
	AddRefMap    *simpleTracerRefMapMessage
	RemoveRefMap *simpleTracerRefMapMessage
}

type simpleTracer struct {
	channel       chan *simpleTracerMessage
	mutex         sync.RWMutex
	referenceMaps map[string][]*refutils.RefMap
	stackTraces   map[string]map[refutils.ID][]byte
}

var st *simpleTracer

func newSimpleTracer() *simpleTracer {
	t := &simpleTracer{
		channel:       make(chan *simpleTracerMessage),
		referenceMaps: map[string][]*refutils.RefMap{},
	}
	st = t
	return t
}

func (t *simpleTracer) Start() {
	go func() {
		i := 0
		for m := range t.channel {
			i++
			log.Println("received message, locking", i)
			t.mutex.Lock()
			if m.Add != nil {
				t.add(m.Add.Ref, m.Add.StackTrace)
			} else if m.Remove != nil {
				t.remove(m.Remove)
			} else if m.AddRefMap != nil {
				t.addRefMap(m.AddRefMap.Name, m.AddRefMap.RefMap)
			} else if m.RemoveRefMap != nil {
				t.removeRefMap(m.RemoveRefMap.Name, m.RemoveRefMap.RefMap)
			}
			t.mutex.Unlock()
			log.Println("finished message, unlocking", i)
		}
	}()
}

func (t *simpleTracer) Stop() {
	close(t.channel)
}

func (t *simpleTracer) EnableAllocationStackTraces() {
	t.stackTraces = map[string]map[refutils.ID][]byte{}
}

func (t *simpleTracer) DisableAllocationStackTraces() {
	t.stackTraces = nil
}

func (t *simpleTracer) weakReferenceMapForReference(value refutils.Ref) (string, *refutils.RefMap) {
	structType := reflect.ValueOf(value).Elem().Type()
	name := structType.Name()

	if _, ok := t.referenceMaps[name]; !ok {
		t.referenceMaps[name] = []*refutils.RefMap{refutils.NewWeakRefMap("v8-simple-tracer")}
	}
	m := t.referenceMaps[name][0]

	return name, m
}

func (t *simpleTracer) add(value refutils.Ref, stack []byte) {
	name, rm := t.weakReferenceMapForReference(value)
	id := rm.Ref(value)

	if t.stackTraces != nil {
		if _, ok := t.stackTraces[name]; !ok {
			t.stackTraces[name] = map[refutils.ID][]byte{}
		}
		t.stackTraces[name][id] = stack
	}
}

func (t *simpleTracer) Add(value refutils.Ref) {
	t.channel <- &simpleTracerMessage{Add: &simpleTracerAddMessage{value, nil}}
}

func (t *simpleTracer) remove(value refutils.Ref) {
	name, rm := t.weakReferenceMapForReference(value)

	if t.stackTraces != nil {
		if _, ok := t.stackTraces[name]; ok {
			i := rm.GetID(value)
			if _, ok := t.stackTraces[name][i]; ok {
				delete(t.stackTraces[name], i)
			}
		}
	}

	rm.Unref(value)
}

func (t *simpleTracer) Remove(value refutils.Ref) {
	t.channel <- &simpleTracerMessage{Remove: value}
}

func (t *simpleTracer) addRefMap(name string, refMap *refutils.RefMap) {
	if _, ok := t.referenceMaps[name]; !ok {
		t.referenceMaps[name] = []*refutils.RefMap{}
	}

	t.referenceMaps[name] = append(t.referenceMaps[name], refMap)
}

func (t *simpleTracer) AddRefMap(name string, refMap *refutils.RefMap) {
	t.channel <- &simpleTracerMessage{AddRefMap: &simpleTracerRefMapMessage{name, refMap}}
}

func (t *simpleTracer) removeRefMap(name string, refMap *refutils.RefMap) {
	if _, ok := t.referenceMaps[name]; !ok {
		return
	}

	for i, r := range t.referenceMaps[name] {
		if r == refMap {
			t.referenceMaps[name] = append(t.referenceMaps[name][:i], t.referenceMaps[name][i+1:]...)
			break
		}
	}

	if len(t.referenceMaps[name]) == 0 {
		delete(t.referenceMaps, name)
	}
}

func (t *simpleTracer) RemoveRefMap(name string, refMap *refutils.RefMap) {
	t.channel <- &simpleTracerMessage{RemoveRefMap: &simpleTracerRefMapMessage{name, refMap}}
}

func sortedMapStringUint64(m map[string]uint64, f func(k string, v uint64)) {
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
	// for _, isolate := range t.isolates {
	// 	if err := isolate.lock(); err != nil {
	// 		return
	// 	} else {
	// 		defer isolate.unlock()
	// 	}
	// }
	t.mutex.Lock()
	defer t.mutex.Unlock()

	// runtime.GC()
	// for _, isolate := range isolates.Refs() {
	// 	//if err := isolate.lock(); err == nil {
	// 		isolate.RequestGarbageCollectionForTesting()
	// 		//defer isolate.unlock()
	// 	//}
	// }

	// t.mutex.Lock()
	// defer t.mutex.Unlock()
	// defer t.referenceMapsMutex.RUnlock()

	stats := map[string]uint64{}

	fmt.Fprintf(w, "%s\n", strings.Repeat("=", 80))
	fmt.Fprintf(w, "V8 Golang Tracer Dump\n%s\n", time.Now())
	fmt.Fprintf(w, "%s\n", strings.Repeat("-", 80))

	for name, referenceMaps := range t.referenceMaps {
		stats[name] = 0
		for _, referenceMap := range referenceMaps {
			stats[name] += uint64(referenceMap.Length())
		}
	}

	sortedMapStringUint64(stats, func(name string, value uint64) {
		fmt.Fprintf(w, "%s: %d\n", name, value)
	})

	// stats = map[string]uint64{}
	// fmt.Fprintf(w, "%s\n", strings.Repeat("-", 80))
	// fmt.Fprintf(w, "V8 Isolate Heap Statistics:\n\n")
	//
	// stats["total heap size"] = 0
	// stats["total heap size executable"] = 0
	// stats["total physical size"] = 0
	// stats["total available size"] = 0
	// stats["used heap size"] = 0
	// stats["heap size limit"] = 0
	// stats["malloced memory"] = 0
	// stats["peak malloced memory"] = 0
	//
	// for _, isolate := range t.isolates {
	// 	hs := isolate.GetHeapStatistics()
	// 	stats["total heap size"] += hs.TotalHeapSize
	// 	stats["total heap size executable"] += hs.TotalHeapSizeExecutable
	// 	stats["total physical size"] += hs.TotalPhysicalSize
	// 	stats["total available size"] += hs.TotalAvailableSize
	// 	stats["used heap size"] += hs.UsedHeapSize
	// 	stats["heap size limit"] += hs.HeapSizeLimit
	// 	stats["malloced memory"] += hs.MallocedMemory
	// 	stats["peak malloced memory"] += hs.PeakMallocedMemory
	// }
	//
	// sortedMapStringUint64(stats, func(name string, value uint64) {
	// 	fmt.Fprintf(w, "%s: %d\n", name, value)
	// })

	if allocations {
		fmt.Fprintf(w, "%s\n", strings.Repeat("-", 80))

		for name, maps := range t.referenceMaps {
			for _, rm := range maps {
				for id, ref := range rm.Refs() {
					if id == 0 {
						fmt.Fprintf(w, "  0x%08x (%s): %s\n", id, name, "(defunct)")
						continue
					}

					fmt.Fprintf(w, "  0x%08x (%s): %#v\n", id, name, ref)
					if t.stackTraces != nil {
						if traces, ok := t.stackTraces[name]; ok {
							if b, ok := traces[id]; ok {
								fmt.Fprintf(w, "    %s\n", strings.Replace(string(b), "\n", "\n    ", -1))
							}
						}
					}
				}
			}
		}
	}

	fmt.Fprintf(w, "%s\n", strings.Repeat("-", 80))

}
