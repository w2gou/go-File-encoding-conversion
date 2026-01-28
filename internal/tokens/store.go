package tokens

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"sync"
	"time"
)

type Item struct {
	Token     string
	Kind      string
	FileID    string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type Options struct {
	CleanupInterval time.Duration
	Rand            io.Reader
}

type Store struct {
	mu    sync.Mutex
	items map[string]Item

	rand io.Reader

	stopOnce sync.Once
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

func NewStore(opt Options) *Store {
	r := opt.Rand
	if r == nil {
		r = rand.Reader
	}

	s := &Store{
		items:  make(map[string]Item),
		rand:   r,
		stopCh: make(chan struct{}),
	}

	if opt.CleanupInterval > 0 {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			ticker := time.NewTicker(opt.CleanupInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					s.pruneExpired(time.Now())
				case <-s.stopCh:
					return
				}
			}
		}()
	}

	return s
}

func (s *Store) Close() {
	s.stopOnce.Do(func() { close(s.stopCh) })
	s.wg.Wait()
}

func (s *Store) Create(kind string, fileID string, ttl time.Duration) (Item, error) {
	return s.CreateAt(time.Now(), kind, fileID, ttl)
}

func (s *Store) CreateAt(now time.Time, kind string, fileID string, ttl time.Duration) (Item, error) {
	if kind == "" {
		return Item{}, fmt.Errorf("%w: kind is required", ErrInvalidInput)
	}
	if ttl <= 0 {
		return Item{}, fmt.Errorf("%w: ttl must be > 0", ErrInvalidInput)
	}
	if now.IsZero() {
		now = time.Now()
	}

	token, err := s.newToken()
	if err != nil {
		return Item{}, err
	}

	it := Item{
		Token:     token,
		Kind:      kind,
		FileID:    fileID,
		CreatedAt: now.UTC(),
		ExpiresAt: now.Add(ttl).UTC(),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneExpiredLocked(now)
	s.items[token] = it
	return it, nil
}

func (s *Store) Peek(token string) (Item, error) {
	return s.PeekAt(time.Now(), token)
}

func (s *Store) PeekAt(now time.Time, token string) (Item, error) {
	if token == "" {
		return Item{}, fmt.Errorf("%w: token is required", ErrInvalidInput)
	}
	if now.IsZero() {
		now = time.Now()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneExpiredLocked(now)

	it, ok := s.items[token]
	if !ok {
		return Item{}, ErrNotFound
	}
	return it, nil
}

func (s *Store) Consume(token string, kind string) (Item, error) {
	return s.ConsumeAt(time.Now(), token, kind)
}

func (s *Store) ConsumeAt(now time.Time, token string, kind string) (Item, error) {
	if token == "" {
		return Item{}, fmt.Errorf("%w: token is required", ErrInvalidInput)
	}
	if kind == "" {
		return Item{}, fmt.Errorf("%w: kind is required", ErrInvalidInput)
	}
	if now.IsZero() {
		now = time.Now()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneExpiredLocked(now)

	it, ok := s.items[token]
	if !ok {
		return Item{}, ErrNotFound
	}
	if it.Kind != kind {
		return Item{}, ErrKindMismatch
	}
	delete(s.items, token)
	return it, nil
}

func (s *Store) pruneExpired(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneExpiredLocked(now)
}

func (s *Store) pruneExpiredLocked(now time.Time) {
	for k, it := range s.items {
		if !now.Before(it.ExpiresAt) {
			delete(s.items, k)
		}
	}
}

func (s *Store) newToken() (string, error) {
	var b [32]byte
	if _, err := io.ReadFull(s.rand, b[:]); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

