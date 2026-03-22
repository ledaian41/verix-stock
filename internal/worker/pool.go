package worker

import "sync"

// Pool is a bounded goroutine pool.
type Pool struct {
    sem chan struct{}
    wg  sync.WaitGroup
}

// NewPool creates a pool with fixed concurrency.
func NewPool(concurrency int) *Pool {
    return &Pool{sem: make(chan struct{}, concurrency)}
}

// Submit runs f in a goroutine, blocking if pool is full.
func (p *Pool) Submit(f func()) {
    p.sem <- struct{}{}
    p.wg.Add(1)
    go func() {
        defer func() { <-p.sem; p.wg.Done() }()
        f()
    }()
}

// Wait blocks until all current jobs are finished.
func (p *Pool) Wait() { p.wg.Wait() }
