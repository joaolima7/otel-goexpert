package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
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
	weatherApiKey string
	tracer        trace.Tracer
)

type CepRequest struct {
	Cep string `json:"cep"`
}

type ViaCepResponse struct {
	Cep         string `json:"cep"`
	Logradouro  string `json:"logradouro"`
	Complemento string `json:"complemento"`
	Bairro      string `json:"bairro"`
	Localidade  string `json:"localidade"`
	Uf          string `json:"uf"`
	Ibge        string `json:"ibge"`
	Gia         string `json:"gia"`
	Ddd         string `json:"ddd"`
	Siafi       string `json:"siafi"`
	Erro        bool   `json:"erro,omitempty"`
}

type WeatherResponse struct {
	Location struct {
		Name string `json:"name"`
	} `json:"location"`
	Current struct {
		TempC float64 `json:"temp_c"`
	} `json:"current"`
}

type WeatherResult struct {
	City  string  `json:"city"`
	TempC float64 `json:"temp_C"`
	TempF float64 `json:"temp_F"`
	TempK float64 `json:"temp_K"`
}

type ErrorResponse struct {
	Message string `json:"message"`
}

func main() {
	weatherApiKey = getEnv("WEATHER_API_KEY", "bfbdabb82902462aaf4190220252008")
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

	r.Post("/weather", handleWeatherRequest)

	port := getEnv("PORT", "8081")
	fmt.Printf("Service B listening on port %s...\n", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}

func handleWeatherRequest(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "handle_weather_request")
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

	location, err := getCepInfo(ctx, req.Cep)
	if err != nil {
		if errors.Is(err, ErrCepNotFound) {
			respondWithError(w, http.StatusNotFound, "can not find zipcode", ctx)
			return
		}
		respondWithError(w, http.StatusInternalServerError, "internal server error", ctx)
		return
	}

	weather, err := getWeatherInfo(ctx, location)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "internal server error", ctx)
		return
	}

	tempC := weather.Current.TempC
	tempF := celsiusToFahrenheit(tempC)
	tempK := celsiusToKelvin(tempC)

	result := WeatherResult{
		City:  location,
		TempC: tempC,
		TempF: tempF,
		TempK: tempK,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}

var (
	ErrCepNotFound = errors.New("cep not found")
)

func getCepInfo(ctx context.Context, cep string) (string, error) {
	ctx, span := tracer.Start(ctx, "get_cep_info")
	defer span.End()

	url := fmt.Sprintf("https://viacep.com.br/ws/%s/json/", cep)
	client := http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error calling ViaCEP API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code from ViaCEP: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body: %w", err)
	}

	var cepInfo ViaCepResponse
	if err := json.Unmarshal(body, &cepInfo); err != nil {
		return "", fmt.Errorf("error unmarshaling ViaCEP response: %w", err)
	}

	if cepInfo.Erro {
		return "", ErrCepNotFound
	}

	return cepInfo.Localidade, nil
}

func getWeatherInfo(ctx context.Context, city string) (*WeatherResponse, error) {
	ctx, span := tracer.Start(ctx, "get_weather_info")
	defer span.End()

	encodedCity := url.QueryEscape(city)
	url := fmt.Sprintf("https://api.weatherapi.com/v1/current.json?key=%s&q=%s&aqi=no", weatherApiKey, encodedCity)
	client := http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		log.Printf("Error creating request to Weather API: %v", err)
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error calling Weather API: %v", err)
		return nil, fmt.Errorf("error calling Weather API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Weather API error: status=%d, body=%s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("unexpected status code from Weather API: %d, body: %s", resp.StatusCode, string(body))
	}

	var weather WeatherResponse
	if err := json.NewDecoder(resp.Body).Decode(&weather); err != nil {
		log.Printf("Error decoding Weather API response: %v", err)
		return nil, fmt.Errorf("error decoding Weather API response: %w", err)
	}

	return &weather, nil
}

// Funções auxiliares para conversão de temperatura
func celsiusToFahrenheit(celsius float64) float64 {
	return celsius*1.8 + 32
}

func celsiusToKelvin(celsius float64) float64 {
	return celsius + 273
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
			semconv.ServiceNameKey.String("service-b"),
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
	tracer = tp.Tracer("service-b")

	return tp, nil
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
