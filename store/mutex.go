package store

import (
	"io"
	"sync"
)

type KeyMutex struct {
	entries map[string]*sync.RWMutex
	mutex   sync.Mutex
}

func NewKeyMutex() *KeyMutex {
	return &KeyMutex{
		entries: map[string]*sync.RWMutex{},
	}
}

func (m *KeyMutex) mutexFor(key string) *sync.RWMutex {
	if mutex, ok := m.entries[key]; ok {
		return mutex
	} else {
		mutex := &sync.RWMutex{}
		m.entries[key] = mutex
		return mutex
	}
}

func (m *KeyMutex) KeyLock(key string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// log.Println(runtime.Caller(2))
	// t := time.Now()
	// log.Printf("waiting for write lock on %q", key)
	m.mutexFor(key).Lock()
	// log.Printf("acquired write lock on %q (took %s)", key, time.Now().Sub(t))
}

func (m *KeyMutex) KeyUnlock(key string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// log.Printf("releasing write lock on %q", key)
	m.mutexFor(key).Unlock()
}

func (m *KeyMutex) KeyRLock(key string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// log.Println(runtime.Caller(2))
	// t := time.Now()
	// log.Printf("waiting for read lock on %q", key)
	m.mutexFor(key).RLock()
	// log.Printf("acquired read lock on %q (took %s)", key, time.Now().Sub(t))
}

func (m *KeyMutex) KeyRUnlock(key string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// log.Printf("releasing read lock on %q", key)
	m.mutexFor(key).RUnlock()
}

func (m *KeyMutex) KeyRCloser(key string, rc io.ReadCloser) io.ReadCloser {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	return &MutexReadCloser{rc, m.mutexFor(key)}
}

func (m *KeyMutex) KeyWCloser(key string, wc io.WriteCloser) io.WriteCloser {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	return &MutexWriteCloser{wc, m.mutexFor(key)}
}

type MutexReadCloser struct {
	io.ReadCloser
	mutex *sync.RWMutex
}

func (m *MutexReadCloser) Close() error {
	// log.Printf("releasing read lock")
	defer m.mutex.RUnlock()
	if err := m.ReadCloser.Close(); err != nil {
		return err
	}
	return nil
}

type MutexWriteCloser struct {
	io.WriteCloser
	mutex *sync.RWMutex
}

func (m *MutexWriteCloser) Close() error {
	// log.Printf("releasing write lock")
	defer m.mutex.Unlock()
	if err := m.WriteCloser.Close(); err != nil {
		return err
	}
	return nil
}
