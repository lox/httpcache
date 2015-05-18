package storage

import (
	"container/list"

	"sync"
)

type lruEntry struct {
	Storable
	key string
}

func (e *lruEntry) Size() uint64 {
	return e.Storable.Size() + uint64(len(e.key))
}

// LRUList provides a map of key to entries, ordered by last access
type LRUList struct {
	sync.Mutex
	list  *list.List
	table map[string]*list.Element
}

// NewLRUList returns a new LRUList
func NewLRUList() *LRUList {
	return &LRUList{
		list:  list.New(),
		table: make(map[string]*list.Element),
	}
}

// Get returns an entry by key, or nil and false. The entry
// is marked as the most recently used.
func (l *LRUList) Get(key string) (Storable, bool) {
	l.Lock()
	defer l.Unlock()

	if ent, ok := l.table[key]; ok {
		l.list.MoveToFront(ent)
		return ent.Value.(*lruEntry).Storable, true
	}

	return nil, false
}

// Oldest gets the most least recently used entry and key
func (l *LRUList) Oldest() (Storable, string) {
	l.Lock()
	defer l.Unlock()

	if e := l.list.Back(); e != nil {
		return e.Value.(*lruEntry).Storable, e.Value.(*lruEntry).key
	}

	return nil, ""
}

// Add adds an entry to the list as the most recently used
func (l *LRUList) Add(key string, r Storable) {
	l.Lock()
	defer l.Unlock()

	e := &lruEntry{r, key}
	l.table[key] = l.list.PushFront(e)
}

// Delete removes an item from the list by key
func (l *LRUList) Delete(key string) {
	l.Lock()
	defer l.Unlock()

	if ent, ok := l.table[key]; ok {
		l.list.Remove(ent)
		le := ent.Value.(*lruEntry)
		delete(l.table, le.key)
	}
}

// Keys returns a slice of the keys in the list in order of
// most recently used to least recently used
func (l *LRUList) Keys() []string {
	l.Lock()
	defer l.Unlock()

	keys := make([]string, l.list.Len())
	i := 0

	for e := l.list.Front(); e != nil; e = e.Next() {
		keys[i] = e.Value.(*lruEntry).key
		i++
	}

	return keys
}

// Len returns the number of items in the list.
func (l *LRUList) Len() int {
	l.Lock()
	defer l.Unlock()
	return l.list.Len()
}

const UnboundedCapacity = 0

// CappedLRUList is a size-constrained collection of resources that
// will remove the oldest resources to make space for new ones automatically
type CappedLRUList struct {
	*LRUList
	sync.Mutex
	capacity, size uint64
}

// NewCappedLRUList returns a new CappedLRUList with the given capacity in bytes,
// or zero for unbounded
func NewCappedLRUList(capacity uint64) *CappedLRUList {
	return &CappedLRUList{LRUList: NewLRUList(), capacity: capacity, size: 0}
}

// If the storage is over capacity, clear elements (starting at the end of the list)
// until it is back under capacity.
func (cl *CappedLRUList) trim() {
	if cl.capacity != UnboundedCapacity {
		for cl.size > cl.capacity {
			_, key := cl.Oldest()
			cl.Delete(key)
		}
	}
}

func (cl *CappedLRUList) Get(key string) (Storable, bool) {
	return cl.LRUList.Get(key)
}

func (cl *CappedLRUList) Add(key string, s Storable) {
	cl.LRUList.Add(key, s)
	cl.size += (s.Size() + uint64(len(key)))
	cl.trim()
}

func (cl *CappedLRUList) Delete(key string) {
	s, exists := cl.LRUList.Get(key)
	if exists {
		cl.LRUList.Delete(key)
		cl.size -= (s.Size() + uint64(len(key)))
	}
}
