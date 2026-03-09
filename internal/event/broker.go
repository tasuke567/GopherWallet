package event

import "context"

// Publisher abstracts the message broker so we can swap NATS/Kafka/RabbitMQ.
type Publisher interface {
	Publish(ctx context.Context, subject string, data []byte) error
	Close() error
}

// Subscriber abstracts message consumption.
type Subscriber interface {
	Subscribe(subject string, handler func(data []byte)) error
	Close() error
}
