package messaging

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
)

type NATSClient struct {
	conn   *nats.Conn
	logger *slog.Logger
}

func NewNATSClient(url string, logger *slog.Logger) (*NATSClient, error) {
	opts := []nats.Option{
		nats.Name("gopher-wallet"),
		nats.ReconnectWait(2 * time.Second),
		nats.MaxReconnects(10),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			logger.Warn("NATS disconnected", "error", err)
		}),
		nats.ReconnectHandler(func(_ *nats.Conn) {
			logger.Info("NATS reconnected")
		}),
	}

	conn, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, fmt.Errorf("connect to NATS: %w", err)
	}

	logger.Info("connected to NATS", "url", url)
	return &NATSClient{conn: conn, logger: logger}, nil
}

func (c *NATSClient) Publish(_ context.Context, subject string, data []byte) error {
	if err := c.conn.Publish(subject, data); err != nil {
		return fmt.Errorf("publish to %s: %w", subject, err)
	}
	return nil
}

func (c *NATSClient) Subscribe(subject string, handler func(data []byte)) error {
	_, err := c.conn.Subscribe(subject, func(msg *nats.Msg) {
		handler(msg.Data)
	})
	if err != nil {
		return fmt.Errorf("subscribe to %s: %w", subject, err)
	}
	c.logger.Info("subscribed to subject", "subject", subject)
	return nil
}

func (c *NATSClient) Close() error {
	c.conn.Close()
	return nil
}
