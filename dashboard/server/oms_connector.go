package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

// OMSConnector connects to the real OMS backend
type OMSConnector struct {
	nc     *nats.Conn
	server *Server
	logger *zap.Logger
}

// NewOMSConnector creates a new OMS connector
func NewOMSConnector(natsURL string, server *Server, logger *zap.Logger) (*OMSConnector, error) {
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	return &OMSConnector{
		nc:     nc,
		server: server,
		logger: logger,
	}, nil
}

// Start begins listening to real OMS events
func (c *OMSConnector) Start() error {
	// Subscribe to real OMS order events
	_, err := c.nc.Subscribe("oms.orders.*", func(msg *nats.Msg) {
		c.server.handleOrderUpdate(msg.Data)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to orders: %w", err)
	}

	// Subscribe to real position updates from OMS
	_, err = c.nc.Subscribe("oms.positions.*", func(msg *nats.Msg) {
		c.server.handlePositionUpdate(msg.Data)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to positions: %w", err)
	}

	// Subscribe to real market data from OMS
	_, err = c.nc.Subscribe("oms.market.*", func(msg *nats.Msg) {
		c.server.handleMarketUpdate(msg.Data)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to market data: %w", err)
	}

	// Subscribe to real risk updates
	_, err = c.nc.Subscribe("oms.risk.*", func(msg *nats.Msg) {
		c.server.handleRiskUpdate(msg.Data)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to risk: %w", err)
	}

	// Subscribe to OMS system metrics
	_, err = c.nc.Subscribe("oms.system.metrics", func(msg *nats.Msg) {
		c.server.handleSystemMetrics(msg.Data)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to system metrics: %w", err)
	}

	c.logger.Info("Connected to real OMS backend")
	return nil
}

// Stop closes the connection
func (c *OMSConnector) Stop() {
	if c.nc != nil {
		c.nc.Close()
	}
}