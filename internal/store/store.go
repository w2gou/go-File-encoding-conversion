package store

import (
	"bytes"
	"container/list"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type FileMeta struct {
	ID        string
	Name      string
	CreatedAt time.Time
	SizeBytes int64
	Encoding  string
	IsText    bool
}

type File struct {
	Meta  FileMeta
	Bytes []byte
}

type InMemoryStore struct {
	maxFiles      int
	maxTotalBytes int64

	mu         sync.RWMutex
	byID       map[string]*entry
	byName     map[string]string
	fifo       *list.List
	totalBytes int64
}

type entry struct {
	meta FileMeta
	data []byte
	elem *list.Element
}

type NewParams struct {
	MaxFiles      int
	MaxTotalBytes int64
}

func NewInMemoryStore(p NewParams) (*InMemoryStore, error) {
	if p.MaxFiles <= 0 || p.MaxTotalBytes <= 0 {
		return nil, fmt.Errorf("%w: max_files/max_total_bytes must be > 0", ErrInvalidInput)
	}
	return &InMemoryStore{
		maxFiles:      p.MaxFiles,
		maxTotalBytes: p.MaxTotalBytes,
		byID:          make(map[string]*entry),
		byName:        make(map[string]string),
		fifo:          list.New(),
	}, nil
}

func (s *InMemoryStore) Limits() (maxFiles int, maxTotalBytes int64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.maxFiles, s.maxTotalBytes
}

func (s *InMemoryStore) Stats() (files int, totalBytes int64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.byID), s.totalBytes
}

func (s *InMemoryStore) List() []FileMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]FileMeta, 0, len(s.byID))
	for e := s.fifo.Front(); e != nil; e = e.Next() {
		en := e.Value.(*entry)
		out = append(out, en.meta)
	}
	return out
}

func (s *InMemoryStore) GetMeta(id string) (FileMeta, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	en, ok := s.byID[id]
	if !ok {
		return FileMeta{}, ErrNotFound
	}
	return en.meta, nil
}

// Get returns the stored bytes slice by reference (read-only contract).
// It is safe to keep the returned slice for reading even if ReplaceBytes happens later,
// because ReplaceBytes swaps to a new slice and does not mutate the old one.
func (s *InMemoryStore) Get(id string) (File, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	en, ok := s.byID[id]
	if !ok {
		return File{}, ErrNotFound
	}
	return File{Meta: en.meta, Bytes: en.data}, nil
}

func (s *InMemoryStore) Open(id string) (FileMeta, *bytes.Reader, error) {
	f, err := s.Get(id)
	if err != nil {
		return FileMeta{}, nil, err
	}
	return f.Meta, bytes.NewReader(f.Bytes), nil
}

type AddParams struct {
	Name     string
	Bytes    []byte
	Encoding string
	IsText   bool
	Now      time.Time
}

func (s *InMemoryStore) Add(p AddParams) (FileMeta, error) {
	if p.Now.IsZero() {
		p.Now = time.Now()
	}
	if p.Name == "" {
		return FileMeta{}, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	size := int64(len(p.Bytes))
	if size < 0 {
		return FileMeta{}, fmt.Errorf("%w: invalid bytes", ErrInvalidInput)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.byName[p.Name]; exists {
		return FileMeta{}, ErrNameConflict
	}
	if size > s.maxTotalBytes {
		return FileMeta{}, ErrTooLarge
	}

	if err := s.evictLocked(size); err != nil {
		return FileMeta{}, err
	}

	id := newID()
	meta := FileMeta{
		ID:        id,
		Name:      p.Name,
		CreatedAt: p.Now.UTC(),
		SizeBytes: size,
		Encoding:  p.Encoding,
		IsText:    p.IsText,
	}
	en := &entry{meta: meta, data: p.Bytes}
	en.elem = s.fifo.PushBack(en)

	s.byID[id] = en
	s.byName[p.Name] = id
	s.totalBytes += size

	return meta, nil
}

func (s *InMemoryStore) Delete(id string) (FileMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	en, ok := s.byID[id]
	if !ok {
		return FileMeta{}, ErrNotFound
	}
	s.deleteLocked(en)
	return en.meta, nil
}

func (s *InMemoryStore) Rename(id string, newName string) (FileMeta, error) {
	if newName == "" {
		return FileMeta{}, fmt.Errorf("%w: new name is required", ErrInvalidInput)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	en, ok := s.byID[id]
	if !ok {
		return FileMeta{}, ErrNotFound
	}
	if en.meta.Name == newName {
		return en.meta, nil
	}
	if _, exists := s.byName[newName]; exists {
		return FileMeta{}, ErrNameConflict
	}

	delete(s.byName, en.meta.Name)
	en.meta.Name = newName
	s.byName[newName] = id
	return en.meta, nil
}

type ReplaceParams struct {
	ID       string
	Bytes    []byte
	Encoding string
	IsText   bool
}

func (s *InMemoryStore) ReplaceBytes(p ReplaceParams) (FileMeta, error) {
	if p.ID == "" {
		return FileMeta{}, fmt.Errorf("%w: id is required", ErrInvalidInput)
	}
	newSize := int64(len(p.Bytes))
	if newSize < 0 {
		return FileMeta{}, fmt.Errorf("%w: invalid bytes", ErrInvalidInput)
	}
	if newSize > s.maxTotalBytes {
		return FileMeta{}, ErrTooLarge
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	en, ok := s.byID[p.ID]
	if !ok {
		return FileMeta{}, ErrNotFound
	}

	newTotal := s.totalBytes - en.meta.SizeBytes + newSize
	if newTotal > s.maxTotalBytes {
		return FileMeta{}, ErrReplaceWouldExceed
	}

	s.totalBytes = newTotal
	en.data = p.Bytes
	en.meta.SizeBytes = newSize
	en.meta.Encoding = p.Encoding
	en.meta.IsText = p.IsText
	return en.meta, nil
}

func (s *InMemoryStore) evictLocked(incomingSize int64) error {
	for (len(s.byID) >= s.maxFiles) || (s.totalBytes+incomingSize > s.maxTotalBytes) {
		front := s.fifo.Front()
		if front == nil {
			break
		}
		oldest := front.Value.(*entry)
		s.deleteLocked(oldest)
	}
	if (len(s.byID) >= s.maxFiles) || (s.totalBytes+incomingSize > s.maxTotalBytes) {
		return ErrInsufficientSpace
	}
	return nil
}

func (s *InMemoryStore) deleteLocked(en *entry) {
	delete(s.byID, en.meta.ID)
	delete(s.byName, en.meta.Name)
	s.totalBytes -= en.meta.SizeBytes
	s.fifo.Remove(en.elem)
}

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

