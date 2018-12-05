package v8

import "sync"

type ID uint32

type Reference interface {
	GetID() ID
	SetID(id ID)
}

type entry struct {
	reference Reference
	count     uint32
}

type ReferenceMap struct {
	entries map[ID]*entry
	nextId  ID
	mutex   sync.RWMutex
}

func NewReferenceMap() *ReferenceMap {
	return &ReferenceMap{}
}

func (rm *ReferenceMap) Get(id ID) Reference {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	return rm.entries[id].reference
}

func (rm *ReferenceMap) Ref(r Reference) ID {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	var e *entry
	id := r.GetID()
	if id == 0 {
		rm.nextId++
		id = rm.nextId
	}
	if e = rm.entries[id]; e == nil {
		e = &entry{r, 0}
		rm.entries[id] = e
	}
	e.count++
	return id
}

func (rm *ReferenceMap) Unref(r Reference) {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	id := r.GetID()
	if e := rm.entries[id]; e == nil || e.count <= 1 {
		delete(rm.entries, id)
	} else {
		e.count--
	}
}
