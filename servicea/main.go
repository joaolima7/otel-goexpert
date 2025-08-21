package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	serviceBURL string
	tracer      trace.Tracer
)

type CepRequest struct {
	Cep string `json:"cep"`
}

type ErrorResponse struct {
	Message string `json:"message"`
}

func main() {
	serviceBURL = getEnv("SERVICE_B_URL", "http://serviceb:8081/weather")
	collectorURL := getEnv("OTEL_COLLECTOR_URL", "otel-collector:4317")

	tp, err := initTracer(collectorURL)
	if err != nil {
		log.Fatalf("Failed to initialize tracer: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tp.Shutdown(ctx); err != nil {
			log.Fatalf("Error shutting down tracer provider: %v", err)
		}
	}()

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Post("/cep", handleCepRequest)

	port := getEnv("PORT", "8080")
	fmt.Printf("Service A listening on port %s...\n", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}

func handleCepRequest(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "handle_cep_request")
	defer span.End()

	var req CepRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusUnprocessableEntity, "invalid zipcode", ctx)
		return
	}

	if !isValidCep(req.Cep) {
		respondWithError(w, http.StatusUnprocessableEntity, "invalid zipcode", ctx)
		return
	}

	resp, err := callServiceB(ctx, req.Cep)
	if err != nil {
		if errors.Is(err, ErrCepNotFound) {
			respondWithError(w, http.StatusNotFound, "can not find zipcode", ctx)
			return
		}
		if errors.Is(err, ErrInvalidCep) {
			respondWithError(w, http.StatusUnprocessableEntity, "invalid zipcode", ctx)
			return
		}
		respondWithError(w, http.StatusInternalServerError, "internal server error", ctx)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(resp)
}

var (
	ErrCepNotFound = errors.New("cep not found")
	ErrInvalidCep  = errors.New("invalid cep")
)

func callServiceB(ctx context.Context, cep string) ([]byte, error) {
	ctx, span := tracer.Start(ctx, "call_service_b")
	defer span.End()

	reqBody, err := json.Marshal(map[string]string{"cep": cep})
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", serviceBURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error calling service B: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrCepNotFound
	}
	if resp.StatusCode == http.StatusUnprocessableEntity {
		return nil, ErrInvalidCep
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return body, nil
}

func isValidCep(cep string) bool {
	re := regexp.MustCompile(`^\d{8}$`)
	return re.MatchString(cep)
}

func respondWithError(w http.ResponseWriter, statusCode int, message string, ctx context.Context) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("error_response", trace.WithAttributes(
		semconv.HTTPStatusCodeKey.Int(statusCode),
	))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponse{Message: message})
}

func initTracer(collectorURL string) (*sdktrace.TracerProvider, error) {
	ctx := context.Background()

	conn, err := grpc.DialContext(ctx, collectorURL, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection to collector: %w", err)
	}

	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String("service-a"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	tracer = tp.Tracer("service-a")

	return tp, nil
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
