package tracing

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Config holds tracing configuration
type Config struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	Exporter       string // "jaeger" or "otlp"
	Endpoint       string
	SamplingRate   float64
}

// Tracer wraps OpenTelemetry tracer
type Tracer struct {
	tracer   trace.Tracer
	provider *sdktrace.TracerProvider
}

// NewTracer creates a new tracer with the given configuration
func NewTracer(cfg Config) (*Tracer, error) {
	// Create resource
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
			attribute.String("environment", cfg.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create exporter
	var exporter sdktrace.SpanExporter
	switch cfg.Exporter {
	case "jaeger":
		exporter, err = jaeger.New(
			jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(cfg.Endpoint)),
		)
	case "otlp":
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		conn, err := grpc.DialContext(ctx, cfg.Endpoint,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create gRPC connection: %w", err)
		}
		
		exporter, err = otlptrace.New(
			context.Background(),
			otlptracegrpc.NewClient(otlptracegrpc.WithGRPCConn(conn)),
		)
	default:
		return nil, fmt.Errorf("unsupported exporter: %s", cfg.Exporter)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create exporter: %w", err)
	}

	// Create tracer provider
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(cfg.SamplingRate)),
	)

	// Set global tracer provider
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &Tracer{
		tracer:   provider.Tracer(cfg.ServiceName),
		provider: provider,
	}, nil
}

// Shutdown gracefully shuts down the tracer provider
func (t *Tracer) Shutdown(ctx context.Context) error {
	return t.provider.Shutdown(ctx)
}

// StartSpan starts a new span
func (t *Tracer) StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, name, opts...)
}

// SpanFromContext returns the current span from context
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// AddEvent adds an event to the current span
func AddEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	span := SpanFromContext(ctx)
	span.AddEvent(name, trace.WithAttributes(attrs...))
}

// SetAttributes sets attributes on the current span
func SetAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := SpanFromContext(ctx)
	span.SetAttributes(attrs...)
}

// RecordError records an error on the current span
func RecordError(ctx context.Context, err error, attrs ...attribute.KeyValue) {
	span := SpanFromContext(ctx)
	span.RecordError(err, trace.WithAttributes(attrs...))
}

// OrderSpan creates attributes for order-related spans
func OrderSpan(orderID, exchange, market, orderType, side string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("order.id", orderID),
		attribute.String("exchange.name", exchange),
		attribute.String("market.type", market),
		attribute.String("order.type", orderType),
		attribute.String("order.side", side),
	}
}

// WebSocketSpan creates attributes for WebSocket-related spans
func WebSocketSpan(exchange, streamType string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("exchange.name", exchange),
		attribute.String("websocket.stream_type", streamType),
	}
}

// ExchangeAPISpan creates attributes for Exchange API spans
func ExchangeAPISpan(exchange, endpoint, method string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("exchange.name", exchange),
		attribute.String("http.target", endpoint),
		attribute.String("http.method", method),
	}
}

// RouteSpan creates attributes for routing-related spans
func RouteSpan(strategy string, exchanges []string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("route.strategy", strategy),
		attribute.StringSlice("route.exchanges", exchanges),
	}
}

// TraceMiddleware provides tracing middleware for various components
type TraceMiddleware struct {
	tracer *Tracer
}

// NewTraceMiddleware creates a new trace middleware
func NewTraceMiddleware(tracer *Tracer) *TraceMiddleware {
	return &TraceMiddleware{
		tracer: tracer,
	}
}

// OrderProcessing wraps order processing with tracing
func (tm *TraceMiddleware) OrderProcessing(ctx context.Context, orderID string, fn func(context.Context) error) error {
	ctx, span := tm.tracer.StartSpan(ctx, "order.process",
		trace.WithAttributes(attribute.String("order.id", orderID)),
	)
	defer span.End()

	err := fn(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(trace.Status{
			Code:        trace.StatusCodeError,
			Description: err.Error(),
		})
	}

	return err
}

// WebSocketMessage wraps WebSocket message handling with tracing
func (tm *TraceMiddleware) WebSocketMessage(ctx context.Context, exchange, streamType, messageType string, fn func(context.Context) error) error {
	ctx, span := tm.tracer.StartSpan(ctx, "websocket.message",
		trace.WithAttributes(
			attribute.String("exchange.name", exchange),
			attribute.String("websocket.stream_type", streamType),
			attribute.String("websocket.message_type", messageType),
		),
	)
	defer span.End()

	err := fn(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(trace.Status{
			Code:        trace.StatusCodeError,
			Description: err.Error(),
		})
	}

	return err
}

// ExchangeAPICall wraps Exchange API calls with tracing
func (tm *TraceMiddleware) ExchangeAPICall(ctx context.Context, exchange, endpoint, method string, fn func(context.Context) error) error {
	ctx, span := tm.tracer.StartSpan(ctx, fmt.Sprintf("exchange.api.%s", method),
		trace.WithAttributes(
			attribute.String("exchange.name", exchange),
			attribute.String("http.target", endpoint),
			attribute.String("http.method", method),
		),
	)
	defer span.End()

	err := fn(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(trace.Status{
			Code:        trace.StatusCodeError,
			Description: err.Error(),
		})
	}

	return err
}

// RouteDecision wraps routing decisions with tracing
func (tm *TraceMiddleware) RouteDecision(ctx context.Context, strategy string, fn func(context.Context) ([]string, error)) ([]string, error) {
	ctx, span := tm.tracer.StartSpan(ctx, "route.decision",
		trace.WithAttributes(attribute.String("route.strategy", strategy)),
	)
	defer span.End()

	exchanges, err := fn(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(trace.Status{
			Code:        trace.StatusCodeError,
			Description: err.Error(),
		})
	} else {
		span.SetAttributes(attribute.StringSlice("route.selected_exchanges", exchanges))
	}

	return exchanges, err
}