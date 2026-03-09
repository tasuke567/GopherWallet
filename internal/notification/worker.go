package notification

import (
	"encoding/json"
	"log/slog"

	"github.com/wey/gopher-wallet/internal/event"
)

// Worker subscribes to transfer events and processes notifications.
type Worker struct {
	subscriber event.Subscriber
	logger     *slog.Logger
}

func NewWorker(subscriber event.Subscriber, logger *slog.Logger) *Worker {
	return &Worker{subscriber: subscriber, logger: logger}
}

// Start begins listening for transfer events.
func (w *Worker) Start() error {
	return w.subscriber.Subscribe(event.SubjectTransferCompleted, w.handleTransferCompleted)
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
