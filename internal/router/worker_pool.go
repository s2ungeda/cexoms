package router

import (
	"sync"
)

// WorkerPool manages a pool of workers for parallel execution
type WorkerPool struct {
	size     int
	taskChan chan func()
	wg       sync.WaitGroup
	stopChan chan struct{}
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(size int) *WorkerPool {
	return &WorkerPool{
		size:     size,
		taskChan: make(chan func(), size*2),
		stopChan: make(chan struct{}),
	}
}

// Start starts the worker pool
func (p *WorkerPool) Start() {
	for i := 0; i < p.size; i++ {
		p.wg.Add(1)
		go p.worker()
	}
}

// Stop stops the worker pool
func (p *WorkerPool) Stop() {
	close(p.stopChan)
	p.wg.Wait()
}

// Submit submits a task to the worker pool
func (p *WorkerPool) Submit(task func()) {
	select {
	case p.taskChan <- task:
		// Task submitted
	case <-p.stopChan:
		// Pool is stopping, don't accept new tasks
	}
}

// worker is the main worker loop
func (p *WorkerPool) worker() {
	defer p.wg.Done()
	
	for {
		select {
		case task := <-p.taskChan:
			if task != nil {
				task()
			}
		case <-p.stopChan:
			return
		}
	}
}