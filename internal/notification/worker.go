package notification

import (
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/wey/gopher-wallet/internal/event"
)

// Worker subscribes to transfer events and processes notifications
// using a configurable pool of goroutines for scalability.
type Worker struct {
	subscriber event.Subscriber
	logger     *slog.Logger
	poolSize   int
	jobs       chan []byte
	wg         sync.WaitGroup
}

func NewWorker(subscriber event.Subscriber, logger *slog.Logger, poolSize int) *Worker {
	if poolSize <= 0 {
		poolSize = 4
	}
	return &Worker{
		subscriber: subscriber,
		logger:     logger,
		poolSize:   poolSize,
	}
}

// Start launches the worker pool and subscribes to transfer events.
func (w *Worker) Start() error {
	w.jobs = make(chan []byte, w.poolSize*2)

	for i := 0; i < w.poolSize; i++ {
		w.wg.Add(1)
		go w.processLoop(i)
	}

	return w.subscriber.Subscribe(event.SubjectTransferCompleted, func(data []byte) {
		select {
		case w.jobs <- data:
		default:
			w.logger.Warn("notification worker pool full, dropping event")
		}
	})
}

func (w *Worker) processLoop(id int) {
	defer w.wg.Done()
	w.logger.Info("notification worker started", "worker_id", id)

	for data := range w.jobs {
		w.handleTransferCompleted(data)
	}

	w.logger.Info("notification worker stopped", "worker_id", id)
}

// Stop signals workers to drain the queue and waits for completion.
func (w *Worker) Stop() {
	close(w.jobs)
	w.wg.Wait()
}

func (w *Worker) handleTransferCompleted(data []byte) {
	var evt event.TransferCompleted
	if err := json.Unmarshal(data, &evt); err != nil {
		w.logger.Error("failed to unmarshal transfer event", "error", err)
		return
	}

	w.logger.Info("notification: transfer completed",
		"transaction_id", evt.TransactionID,
		"from", evt.FromAccountID,
		"to", evt.ToAccountID,
		"amount", evt.Amount,
		"currency", evt.Currency,
	)
}
