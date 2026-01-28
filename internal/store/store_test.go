package store

import (
	"testing"
	"time"
)

func TestAddNameConflictRejects(t *testing.T) {
	s, err := NewInMemoryStore(NewParams{MaxFiles: 10, MaxTotalBytes: 1000})
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.Add(AddParams{Name: "a.txt", Bytes: []byte("1"), Now: time.Unix(1, 0)})
	if err != nil {
		t.Fatalf("add 1: %v", err)
	}
	_, err = s.Add(AddParams{Name: "a.txt", Bytes: []byte("2"), Now: time.Unix(2, 0)})
	if err != ErrNameConflict {
		t.Fatalf("expected ErrNameConflict, got %v", err)
	}
}

func TestRenameNameConflictRejectsAndKeepsOriginal(t *testing.T) {
	s, err := NewInMemoryStore(NewParams{MaxFiles: 10, MaxTotalBytes: 1000})
	if err != nil {
		t.Fatal(err)
	}

	a, err := s.Add(AddParams{Name: "a.txt", Bytes: []byte("1"), Now: time.Unix(1, 0)})
	if err != nil {
		t.Fatalf("add a: %v", err)
	}
	_, err = s.Add(AddParams{Name: "b.txt", Bytes: []byte("2"), Now: time.Unix(2, 0)})
	if err != nil {
		t.Fatalf("add b: %v", err)
	}

	_, err = s.Rename(a.ID, "b.txt")
	if err != ErrNameConflict {
		t.Fatalf("expected ErrNameConflict, got %v", err)
	}

	meta, err := s.GetMeta(a.ID)
	if err != nil {
		t.Fatalf("get meta: %v", err)
	}
	if meta.Name != "a.txt" {
		t.Fatalf("expected original name kept, got %q", meta.Name)
	}
}

func TestFIFOEvictionByMaxFiles(t *testing.T) {
	s, err := NewInMemoryStore(NewParams{MaxFiles: 2, MaxTotalBytes: 1000})
	if err != nil {
		t.Fatal(err)
	}

	a, err := s.Add(AddParams{Name: "a.txt", Bytes: []byte("1"), Now: time.Unix(1, 0)})
	if err != nil {
		t.Fatalf("add a: %v", err)
	}
	_, err = s.Add(AddParams{Name: "b.txt", Bytes: []byte("2"), Now: time.Unix(2, 0)})
	if err != nil {
		t.Fatalf("add b: %v", err)
	}
	_, err = s.Add(AddParams{Name: "c.txt", Bytes: []byte("3"), Now: time.Unix(3, 0)})
	if err != nil {
		t.Fatalf("add c: %v", err)
	}

	if _, err := s.GetMeta(a.ID); err != ErrNotFound {
		t.Fatalf("expected oldest evicted (a), got %v", err)
	}
}

func TestFIFOEvictionByMaxTotalBytes(t *testing.T) {
	s, err := NewInMemoryStore(NewParams{MaxFiles: 10, MaxTotalBytes: 3})
	if err != nil {
		t.Fatal(err)
	}

	a, err := s.Add(AddParams{Name: "a.txt", Bytes: []byte("1"), Now: time.Unix(1, 0)})
	if err != nil {
		t.Fatalf("add a: %v", err)
	}
	_, err = s.Add(AddParams{Name: "b.txt", Bytes: []byte("2"), Now: time.Unix(2, 0)})
	if err != nil {
		t.Fatalf("add b: %v", err)
	}
	_, err = s.Add(AddParams{Name: "c.txt", Bytes: []byte("3"), Now: time.Unix(3, 0)})
	if err != nil {
		t.Fatalf("add c: %v", err)
	}

	if _, err := s.GetMeta(a.ID); err != ErrNotFound {
		t.Fatalf("expected a evicted, got %v", err)
	}
}

func TestReplaceBytesWouldExceedRejects(t *testing.T) {
	s, err := NewInMemoryStore(NewParams{MaxFiles: 10, MaxTotalBytes: 3})
	if err != nil {
		t.Fatal(err)
	}

	a, err := s.Add(AddParams{Name: "a.txt", Bytes: []byte("1"), Now: time.Unix(1, 0)})
	if err != nil {
		t.Fatalf("add a: %v", err)
	}
	_, err = s.Add(AddParams{Name: "b.txt", Bytes: []byte("2"), Now: time.Unix(2, 0)})
	if err != nil {
		t.Fatalf("add b: %v", err)
	}

	_, err = s.ReplaceBytes(ReplaceParams{ID: a.ID, Bytes: []byte("111")})
	if err != ErrReplaceWouldExceed {
		t.Fatalf("expected ErrReplaceWouldExceed, got %v", err)
	}

	f, err := s.Get(a.ID)
	if err != nil {
		t.Fatalf("get a: %v", err)
	}
	if string(f.Bytes) != "1" {
		t.Fatalf("expected original bytes kept, got %q", string(f.Bytes))
	}
}

