package store

import (
	"io"
	"log"
	"runtime"
	"sync"
)

var debugMutex bool

type storeMutex struct {
	entries map[string]*keyMutex
	mutex   sync.Mutex
}

func newStoreMutex() *storeMutex {
	return &storeMutex{
		entries: map[string]*keyMutex{},
	}
}

func (m *storeMutex) ForKey(key string) *keyMutex {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if mutex, ok := m.entries[key]; ok {
		return mutex
	} else {
		mutex := &keyMutex{&sync.RWMutex{}, key}
		m.entries[key] = mutex
		return mutex
	}
}

type keyMutex struct {
	*sync.RWMutex
	key string
}

func (m *keyMutex) ReadCloser(rc io.ReadCloser) io.ReadCloser {
	return &mutexReadCloser{rc, m}
}

func (m *keyMutex) WriteCloser(wc io.WriteCloser) io.WriteCloser {
	return &mutexWriteCloser{wc, m}
}

func (m *keyMutex) Lock() {
	if debugMutex {
		log.Printf("Acquiring write lock for %#v", m)
		log.Println(runtime.Caller(2))
	}
	m.RWMutex.Lock()
}

func (m *keyMutex) Unlock() {
	if debugMutex {
		log.Printf("Releasing write lock for %#v", m)
		log.Println(runtime.Caller(2))
	}
	m.RWMutex.Unlock()
}

func (m *keyMutex) RLock() {
	if debugMutex {
		log.Printf("Acquiring read lock for %#v", m)
		log.Println(runtime.Caller(2))
	}
	m.RWMutex.RLock()
}

func (m *keyMutex) RUnlock() {
	if debugMutex {
		log.Printf("Releasing read lock for %#v", m)
		log.Println(runtime.Caller(2))
	}
	m.RWMutex.RUnlock()
}

type mutexReadCloser struct {
	io.ReadCloser
	m *keyMutex
}

func (mrc *mutexReadCloser) Close() error {
	mrc.m.RUnlock()
	return nil
}

type mutexWriteCloser struct {
	io.WriteCloser
	m *keyMutex
}

func (mwc *mutexWriteCloser) Close() error {
	mwc.m.Unlock()
	return nil
}
