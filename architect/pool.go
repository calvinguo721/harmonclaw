// Package architect (pool) provides bounded WorkerPool for skill execution.
package architect

import (
	"errors"
	"log"
	"runtime"
	"sync"
)

var ErrBackpressure = errors.New("skill execution backlogged")

const defaultQueueCap = 64

type Task func()

type WorkerPool struct {
	tasks   chan Task
	workers int
	done    chan struct{}
	wg      sync.WaitGroup
	mu      sync.Mutex
	running bool
}

func NewWorkerPool() *WorkerPool {
	n := runtime.NumCPU()
	if n < 1 {
		n = 1
	}
	return &WorkerPool{
		tasks:   make(chan Task, defaultQueueCap),
		workers: n,
		done:    make(chan struct{}),
	}
}

func (p *WorkerPool) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running {
		return
	}
	p.running = true
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
	log.Printf("architect pool: started %d workers", p.workers)
}

func (p *WorkerPool) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running {
		return
	}
	close(p.done)
	p.wg.Wait()
	p.running = false
	log.Println("architect pool: stopped")
}

func (p *WorkerPool) worker(id int) {
	defer p.wg.Done()
	for {
		select {
		case <-p.done:
			return
		case task, ok := <-p.tasks:
			if !ok {
				return
			}
			p.runTask(id, task)
		}
	}
}

func (p *WorkerPool) runTask(id int, task Task) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("architect pool: worker %d panic: %v — slot restarted", id, r)
		}
	}()
	task()
}

func (p *WorkerPool) Submit(task Task) error {
	select {
	case p.tasks <- task:
		return nil
	default:
		return ErrBackpressure
	}
}

// QueueLen returns the number of tasks waiting in the queue.
func (p *WorkerPool) QueueLen() int {
	return len(p.tasks)
}
