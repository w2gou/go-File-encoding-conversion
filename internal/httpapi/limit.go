package httpapi

// Semaphore 是一个简单的“非阻塞获取”的并发限流器（用于上传/转码等）。
// 目标：在高并发下快速拒绝，避免内存不可控与 GC 抖动。
type Semaphore struct {
	ch chan struct{}
}

func NewSemaphore(n int) *Semaphore {
	if n <= 0 {
		n = 1
	}
	return &Semaphore{ch: make(chan struct{}, n)}
}

func (s *Semaphore) TryAcquire() bool {
	select {
	case s.ch <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *Semaphore) Release() {
	select {
	case <-s.ch:
	default:
	}
}

