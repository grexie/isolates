package v8

import (
	"reflect"
	"sync"
	"unsafe"
)

type id uint32

type reference interface {
	getID(name string) id
	setID(name string, id id)
}

type referenceObject struct {
	ids map[string]id
}

func (o *referenceObject) getID(name string) id {
	if o.ids == nil {
		return 0
	}

	if id, ok := o.ids[name]; !ok {
		return 0
	} else {
		return id
	}
}

func (o *referenceObject) setID(name string, i id) {
	if o.ids == nil {
		o.ids = map[string]id{}
	}

	o.ids[name] = i
}

type entry struct {
	pointer   uintptr
	reference unsafe.Pointer
	count     uint32
}

type referenceMap struct {
	name          string
	entries       map[id]*entry
	referenceType reflect.Type
	nextId        id
	mutex         sync.RWMutex
	strong        bool
}

func newReferenceMap(name string, referenceType reflect.Type) *referenceMap {
	return &referenceMap{
		name:          name,
		entries:       map[id]*entry{},
		referenceType: referenceType,
		strong:        true,
	}
}

func newWeakReferenceMap(name string, referenceType reflect.Type) *referenceMap {
	return &referenceMap{
		name:          name,
		entries:       map[id]*entry{},
		referenceType: referenceType,
		strong:        false,
	}
}

func (rm *referenceMap) References() []reference {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	refs := make([]reference, len(rm.entries))
	i := 0
	for _, entry := range rm.entries {
		refs[i] = reflect.NewAt(rm.referenceType.Elem(), unsafe.Pointer(entry.pointer)).Interface().(reference)
		i++
	}
	return refs
}

func (rm *referenceMap) Length() int {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	return len(rm.entries)
}

func (rm *referenceMap) Get(id id) reference {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	return reflect.NewAt(rm.referenceType.Elem(), unsafe.Pointer(rm.entries[id].pointer)).Interface().(reference)
}

func (rm *referenceMap) Ref(r reference) id {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	var e *entry
	id := r.getID(rm.name)
	if id == 0 {
		rm.nextId++
		id = rm.nextId
		r.setID(rm.name, id)
	}
	if e = rm.entries[id]; e == nil {
		if rm.strong {
			e = &entry{reflect.ValueOf(r).Pointer(), unsafe.Pointer(reflect.ValueOf(r).Pointer()), 0}
			rm.entries[id] = e
		} else {
			e = &entry{reflect.ValueOf(r).Pointer(), nil, 0}
			rm.entries[id] = e
		}
	}
	e.count++
	return id
}

func (rm *referenceMap) Unref(r reference) {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	id := r.getID(rm.name)
	if e, ok := rm.entries[id]; ok && e.count <= 1 {
		delete(rm.entries, id)
	} else if ok {
		e.count--
	}
}

func (rm *referenceMap) Release(r reference) {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	id := r.getID(rm.name)
	if _, ok := rm.entries[id]; ok {
		delete(rm.entries, id)
	}
}
