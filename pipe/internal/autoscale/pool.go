package autoscale

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// ProcessFunc is the core processing function signature.
type ProcessFunc[In, Out any] func(ctx context.Context, in In) ([]Out, error)

// ErrorHandler handles processing errors.
type ErrorHandler func(in any, err error)

// Pool manages a dynamic pool of workers that scales based on load.
type Pool[In, Out any] struct {
	cfg          Config
	fn           ProcessFunc[In, Out]
	errorHandler ErrorHandler
	shutdownErr  error // Error to report when shutdown drops messages

	mu       sync.Mutex
	workers  map[int]*worker
	nextID   int
	stopping bool

	totalWorkers  atomic.Int64
	activeWorkers atomic.Int64

	lastScaleUp   time.Time
	lastScaleDown time.Time

	in   <-chan In
	out  chan Out
	done chan struct{}

	workerWg sync.WaitGroup // Tracks only workers
	scalerWg sync.WaitGroup // Tracks the scaler goroutine
}

type worker struct {
	id         int
	lastActive time.Time
	stopCh     chan struct{}
}

// NewPool creates a new autoscaling worker pool.
func NewPool[In, Out any](
	cfg Config,
	fn ProcessFunc[In, Out],
	errorHandler ErrorHandler,
	shutdownErr error,
) *Pool[In, Out] {
	return &Pool[In, Out]{
		cfg:          cfg,
		fn:           fn,
		errorHandler: errorHandler,
		shutdownErr:  shutdownErr,
		workers:      make(map[int]*worker),
	}
}

// Start begins processing items from the input channel and returns the output channel.
func (p *Pool[In, Out]) Start(ctx context.Context, in <-chan In, bufferSize int) <-chan Out {
	p.in = in
	p.out = make(chan Out, bufferSize)
	p.done = make(chan struct{})

	// Spawn initial workers (MinWorkers)
	for range p.cfg.MinWorkers {
		p.spawnWorker(ctx)
	}

	// Start the scaler goroutine
	p.scalerWg.Add(1)
	go p.runScaler(ctx)

	// Start a goroutine to wait for all workers and close output
	go p.waitForCompletion()

	return p.out
}

// TotalWorkers returns the current number of workers.
func (p *Pool[In, Out]) TotalWorkers() int64 {
	return p.totalWorkers.Load()
}

// ActiveWorkers returns the number of workers currently processing.
func (p *Pool[In, Out]) ActiveWorkers() int64 {
	return p.activeWorkers.Load()
}

func (p *Pool[In, Out]) spawnWorker(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.stopping {
		return
	}

	if p.totalWorkers.Load() >= int64(p.cfg.MaxWorkers) {
		return
	}

	w := &worker{
		id:         p.nextID,
		lastActive: time.Now(),
		stopCh:     make(chan struct{}),
	}
	p.nextID++
	p.workers[w.id] = w
	p.totalWorkers.Add(1)

	p.workerWg.Add(1)
	go p.runWorker(ctx, w)
}

func (p *Pool[In, Out]) stopWorker(id int) {
	p.mu.Lock()
	w, ok := p.workers[id]
	if !ok {
		p.mu.Unlock()
		return
	}
	delete(p.workers, id)
	p.mu.Unlock()

	close(w.stopCh)
	p.totalWorkers.Add(-1)
}

func (p *Pool[In, Out]) runWorker(ctx context.Context, w *worker) {
	defer p.workerWg.Done()

	for {
		select {
		case <-w.stopCh:
			return
		case <-p.done:
			return
		case val, ok := <-p.in:
			if !ok {
				return
			}

			p.activeWorkers.Add(1)
			results, err := p.fn(ctx, val)
			p.activeWorkers.Add(-1)

			p.mu.Lock()
			w.lastActive = time.Now()
			p.mu.Unlock()

			if err != nil {
				p.errorHandler(val, err)
				continue
			}

			for i, r := range results {
				select {
				case p.out <- r:
				case <-p.done:
					// Report this and remaining outputs as dropped
					for _, dropped := range results[i:] {
						p.errorHandler(dropped, p.shutdownErr)
					}
					return
				}
			}
		}
	}
}

func (p *Pool[In, Out]) runScaler(ctx context.Context) {
	defer p.scalerWg.Done()

	ticker := time.NewTicker(p.cfg.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.done:
			return
		case <-ticker.C:
			p.evaluate()
		}
	}
}

func (p *Pool[In, Out]) evaluate() {
	now := time.Now()
	total := p.totalWorkers.Load()
	active := p.activeWorkers.Load()

	// Scale up: all workers busy and below max
	if active >= total &&
		total < int64(p.cfg.MaxWorkers) &&
		now.Sub(p.lastScaleUp) >= p.cfg.ScaleUpCooldown {
		p.spawnWorker(context.Background())
		p.lastScaleUp = now
		return
	}

	// Scale down: find idle workers
	if total > int64(p.cfg.MinWorkers) &&
		now.Sub(p.lastScaleDown) >= p.cfg.ScaleDownCooldown {
		p.mu.Lock()
		var idleWorkerID = -1
		for id, w := range p.workers {
			if now.Sub(w.lastActive) >= p.cfg.ScaleDownAfter {
				idleWorkerID = id
				break
			}
		}
		p.mu.Unlock()

		if idleWorkerID >= 0 {
			p.stopWorker(idleWorkerID)
			p.lastScaleDown = now
		}
	}
}

func (p *Pool[In, Out]) waitForCompletion() {
	// Wait for all workers to finish (they exit when input channel closes)
	p.workerWg.Wait()

	// Signal scaler to stop
	p.mu.Lock()
	if !p.stopping {
		p.stopping = true
		close(p.done)
	}
	p.mu.Unlock()

	// Wait for scaler to finish
	p.scalerWg.Wait()

	// Close output channel
	close(p.out)
}

// Stop signals all workers to stop.
func (p *Pool[In, Out]) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.stopping {
		p.stopping = true
		close(p.done)
	}
}
