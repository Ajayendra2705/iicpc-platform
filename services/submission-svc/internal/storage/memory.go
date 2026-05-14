package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
)

type memoryStore struct {
	mu      sync.RWMutex
	objects map[string][]byte
}

func NewMemory() ObjectStore {
	return &memoryStore{objects: make(map[string][]byte)}
}

func (m *memoryStore) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	m.objects[key] = data
	m.mu.Unlock()
	return "memory://" + key, nil
}

func (m *memoryStore) Get(_ context.Context, key string) (io.ReadCloser, int64, error) {
	m.mu.RLock()
	data, ok := m.objects[key]
	m.mu.RUnlock()
	if !ok {
		return nil, 0, fmt.Errorf("object not found: %s", key)
	}
	return io.NopCloser(bytes.NewReader(data)), int64(len(data)), nil
}
