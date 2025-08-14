package nats

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

// Client wraps NATS connection with OMS-specific functionality
type Client struct {
	conn     *nats.Conn
	js       nats.JetStreamContext
	logger   *logrus.Entry
	config   *Config
}

// Config holds NATS configuration
type Config struct {
	URL        string
	ClusterID  string
	ClientID   string
	Streams    []StreamConfig
}

// StreamConfig defines JetStream configuration
type StreamConfig struct {
	Name      string
	Subjects  []string
	Retention nats.RetentionPolicy
	MaxAge    time.Duration
	MaxMsgs   int64
}

// NewClient creates a new NATS client
func NewClient(config *Config) (*Client, error) {
	logger := logrus.WithField("component", "nats-client")
	
	// Connect to NATS
	opts := []nats.Option{
		nats.Name(config.ClientID),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(time.Second),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			logger.Errorf("NATS disconnected: %v", err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			logger.Info("NATS reconnected")
		}),
		nats.ErrorHandler(func(nc *nats.Conn, sub *nats.Subscription, err error) {
			logger.Errorf("NATS error: %v", err)
		}),
	}
	
	conn, err := nats.Connect(config.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}
	
	// Create JetStream context
	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}
	
	client := &Client{
		conn:   conn,
		js:     js,
		logger: logger,
		config: config,
	}
	
	// Initialize streams
	if err := client.initializeStreams(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to initialize streams: %w", err)
	}
	
	return client, nil
}

// initializeStreams creates JetStream streams if they don't exist
func (c *Client) initializeStreams() error {
	for _, streamConfig := range c.config.Streams {
		config := &nats.StreamConfig{
			Name:      streamConfig.Name,
			Subjects:  streamConfig.Subjects,
			Retention: streamConfig.Retention,
			MaxAge:    streamConfig.MaxAge,
			MaxMsgs:   streamConfig.MaxMsgs,
			Storage:   nats.FileStorage,
			Replicas:  1,
		}
		
		// Check if stream exists
		_, err := c.js.StreamInfo(streamConfig.Name)
		if err == nil {
			// Update existing stream
			_, err = c.js.UpdateStream(config)
			if err != nil {
				return fmt.Errorf("failed to update stream %s: %w", streamConfig.Name, err)
			}
			c.logger.Infof("Updated stream: %s", streamConfig.Name)
		} else {
			// Create new stream
			_, err = c.js.AddStream(config)
			if err != nil {
				return fmt.Errorf("failed to create stream %s: %w", streamConfig.Name, err)
			}
			c.logger.Infof("Created stream: %s", streamConfig.Name)
		}
	}
	
	return nil
}

// Close closes the NATS connection
func (c *Client) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

// PublishOrder publishes an order message
func (c *Client) PublishOrder(exchange, market, symbol, action string, order interface{}) error {
	subject := fmt.Sprintf("orders.%s.%s.%s.%s", action, exchange, market, symbol)
	return c.publish(subject, order)
}

// PublishMarketData publishes market data
func (c *Client) PublishMarketData(exchange, market, symbol string, data interface{}) error {
	subject := fmt.Sprintf("market.%s.%s.%s", exchange, market, symbol)
	return c.publish(subject, data)
}

// PublishPosition publishes position update
func (c *Client) PublishPosition(exchange, market, symbol string, position interface{}) error {
	subject := fmt.Sprintf("positions.%s.%s.%s", exchange, market, symbol)
	return c.publish(subject, position)
}

// publish publishes a message to a subject
func (c *Client) publish(subject string, data interface{}) error {
	msg, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}
	
	_, err = c.js.Publish(subject, msg)
	if err != nil {
		return fmt.Errorf("failed to publish to %s: %w", subject, err)
	}
	
	c.logger.Debugf("Published to %s", subject)
	return nil
}

// SubscribeOrders subscribes to order updates
func (c *Client) SubscribeOrders(exchange, market, symbol string, handler MessageHandler) (*Subscription, error) {
	subject := fmt.Sprintf("orders.*.%s.%s.%s", exchange, market, symbol)
	return c.subscribe(subject, handler)
}

// SubscribeAllOrders subscribes to all order updates
func (c *Client) SubscribeAllOrders(handler MessageHandler) (*Subscription, error) {
	return c.subscribe("orders.>", handler)
}

// SubscribeMarketData subscribes to market data updates
func (c *Client) SubscribeMarketData(exchange, market, symbol string, handler MessageHandler) (*Subscription, error) {
	subject := fmt.Sprintf("market.%s.%s.%s", exchange, market, symbol)
	return c.subscribe(subject, handler)
}

// SubscribeAllMarketData subscribes to all market data
func (c *Client) SubscribeAllMarketData(handler MessageHandler) (*Subscription, error) {
	return c.subscribe("market.>", handler)
}

// subscribe creates a subscription
func (c *Client) subscribe(subject string, handler MessageHandler) (*Subscription, error) {
	sub, err := c.js.Subscribe(subject, func(msg *nats.Msg) {
		if err := handler(msg.Subject, msg.Data); err != nil {
			c.logger.Errorf("Handler error for %s: %v", msg.Subject, err)
		}
		msg.Ack()
	}, nats.Durable(fmt.Sprintf("oms-%s", subject)))
	
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to %s: %w", subject, err)
	}
	
	c.logger.Infof("Subscribed to %s", subject)
	
	return &Subscription{
		sub:    sub,
		logger: c.logger,
	}, nil
}

// MessageHandler processes incoming messages
type MessageHandler func(subject string, data []byte) error

// Subscription wraps NATS subscription
type Subscription struct {
	sub    *nats.Subscription
	logger *logrus.Entry
}

// Unsubscribe removes the subscription
func (s *Subscription) Unsubscribe() error {
	if err := s.sub.Unsubscribe(); err != nil {
		return fmt.Errorf("failed to unsubscribe: %w", err)
	}
	s.logger.Info("Unsubscribed")
	return nil
}

// Subject parsing utilities

// ParseOrderSubject parses order subject format: orders.{action}.{exchange}.{market}.{symbol}
func ParseOrderSubject(subject string) (action, exchange, market, symbol string, err error) {
	// Use strings.Split instead of fmt.Sscanf
	parts := strings.Split(subject, ".")
	if len(parts) < 5 {
		return "", "", "", "", fmt.Errorf("invalid order subject format: %s", subject)
	}
	return parts[1], parts[2], parts[3], parts[4], nil
}

// ParseMarketSubject parses market subject format: market.{exchange}.{market}.{symbol}
func ParseMarketSubject(subject string) (exchange, market, symbol string, err error) {
	// Use strings.Split instead of fmt.Sscanf
	parts := strings.Split(subject, ".")
	if len(parts) < 4 {
		return "", "", "", fmt.Errorf("invalid market subject format: %s", subject)
	}
	return parts[1], parts[2], parts[3], nil
}

// PublishSystem publishes system messages
func (c *Client) PublishSystem(component, event string, data interface{}) error {
	subject := fmt.Sprintf("system.%s.%s", component, event)
	return c.publish(subject, data)
}

// PublishBalance publishes balance updates
func (c *Client) PublishBalance(exchange, market string, balance interface{}) error {
	subject := fmt.Sprintf("balance.%s.%s", exchange, market)
	return c.publish(subject, balance)
}

// Subscribe creates a generic subscription
func (c *Client) Subscribe(subject string, handler func(msg *nats.Msg)) (*nats.Subscription, error) {
	return c.conn.Subscribe(subject, handler)
}