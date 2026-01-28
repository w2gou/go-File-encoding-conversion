package tokens

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConsumeOneTime(t *testing.T) {
	s := NewStore(Options{})
	defer s.Close()

	now := time.Unix(100, 0)
	it, err := s.CreateAt(now, "download", "file1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.ConsumeAt(now, it.Token, "download"); err != nil {
		t.Fatalf("consume 1: %v", err)
	}
	if _, err := s.ConsumeAt(now, it.Token, "download"); err != ErrNotFound {
		t.Fatalf("consume 2 expected ErrNotFound, got %v", err)
	}
}

func TestPeekDoesNotConsume(t *testing.T) {
	s := NewStore(Options{})
	defer s.Close()

	now := time.Unix(100, 0)
	it, err := s.CreateAt(now, "bridge-upload", "", time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.PeekAt(now, it.Token); err != nil {
		t.Fatalf("peek: %v", err)
	}
	if _, err := s.ConsumeAt(now, it.Token, "bridge-upload"); err != nil {
		t.Fatalf("consume: %v", err)
	}
}

func TestExpired(t *testing.T) {
	s := NewStore(Options{})
	defer s.Close()

	now := time.Unix(100, 0)
	it, err := s.CreateAt(now, "download", "file1", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.PeekAt(now.Add(3*time.Second), it.Token); err != ErrNotFound {
		t.Fatalf("peek after expiry expected ErrNotFound, got %v", err)
	}
	if _, err := s.ConsumeAt(now.Add(3*time.Second), it.Token, "download"); err != ErrNotFound {
		t.Fatalf("consume after expiry expected ErrNotFound, got %v", err)
	}
}

func TestConsumeKindMismatch(t *testing.T) {
	s := NewStore(Options{})
	defer s.Close()

	now := time.Unix(100, 0)
	it, err := s.CreateAt(now, "download", "file1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.ConsumeAt(now, it.Token, "bridge-download"); err != ErrKindMismatch {
		t.Fatalf("expected ErrKindMismatch, got %v", err)
	}

	// still present and consumable with correct kind
	if _, err := s.ConsumeAt(now, it.Token, "download"); err != nil {
		t.Fatalf("consume correct kind: %v", err)
	}
}

func TestConcurrentConsumeOnlyOneWins(t *testing.T) {
	s := NewStore(Options{})
	defer s.Close()

	now := time.Unix(100, 0)
	it, err := s.CreateAt(now, "download", "file1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	var okCount int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.ConsumeAt(now, it.Token, "download"); err == nil {
				atomic.AddInt32(&okCount, 1)
			}
		}()
	}
	wg.Wait()

	if okCount != 1 {
		t.Fatalf("expected exactly 1 successful consume, got %d", okCount)
	}
}

