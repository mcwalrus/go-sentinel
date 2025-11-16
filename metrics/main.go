package main

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/eiannone/keyboard"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// Create metric Vecs with no labels
	requestCounterVec = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "app_requests_total",
			Help: "Total number of requests",
		},
		[]string{}, // No labels
	)

	activeConnectionsVec = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "app_active_connections",
			Help: "Number of active connections",
		},
		[]string{}, // No labels
	)

	responseTimeVec = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "app_response_time_seconds",
			Help:    "Response time in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{}, // No labels
	)

	// Get the individual metrics by passing no labels
	requestCounter     prometheus.Counter
	activeConnections  prometheus.Gauge
	responseTimeMetric prometheus.Observer
)

func init() {
	// Register the Vecs
	prometheus.MustRegister(requestCounterVec)
	prometheus.MustRegister(activeConnectionsVec)
	prometheus.MustRegister(responseTimeVec)

	// Transform Vecs into individual metrics by passing no labels
	requestCounter = requestCounterVec.WithLabelValues()
	activeConnections = activeConnectionsVec.WithLabelValues()
	responseTimeMetric = responseTimeVec.WithLabelValues()

	// Initialize some metrics
	activeConnections.Set(0)
}

func main() {
	// Start HTTP server in a goroutine
	go startHTTPServer()

	fmt.Println("Prometheus metrics server started on :8080")
	fmt.Println("Metrics available at: http://localhost:8080/metrics")
	fmt.Println("\nKey bindings:")
	fmt.Println("  [+] or [=] - Increment request counter")
	fmt.Println("  [-] or [_] - Decrement request counter (if possible)")
	fmt.Println("  [u] or [↑] - Increment active connections")
	fmt.Println("  [d] or [↓] - Decrement active connections")
	fmt.Println("  [r]        - Record a sample response time")
	fmt.Println("  [s]        - Show current metric values")
	fmt.Println("  [q]        - Quit")
	fmt.Println("\nPress keys to update metrics...")

	// Try to use keyboard library for better key detection
	if err := keyboard.Open(); err == nil {
		defer keyboard.Close()
		handleKeyboardInput()
	} else {
		// Fallback to simple input
		fmt.Println("\n(Note: Using simple input mode. Type command and press Enter)")
		handleSimpleInput()
	}
}

func startHTTPServer() {
	http.Handle("/metrics", promhttp.Handler())

	// Optional: Add a simple index page
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `
			<html>
			<head><title>Metrics Demo</title></head>
			<body>
				<h1>Prometheus Metrics Demo</h1>
				<p>This application demonstrates Prometheus metric Vecs with no labels.</p>
				<p><a href="/metrics">View Metrics</a></p>
				<p>Use the terminal to interact with metrics via key bindings.</p>
			</body>
			</html>
		`)
	})

	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleKeyboardInput() {
	for {
		char, key, err := keyboard.GetKey()
		if err != nil {
			log.Printf("Error reading key: %v", err)
			continue
		}

		processKey(char, key)
	}
}

func handleSimpleInput() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		if len(input) == 0 {
			continue
		}

		char := rune(input[0])
		processKey(char, keyboard.Key(0))
	}
}

func processKey(char rune, key keyboard.Key) {
	switch {
	case char == '+' || char == '=':
		requestCounter.Inc()
		fmt.Println("✓ Request counter incremented")

	case char == '-' || char == '_':
		// Counters can only go up in Prometheus, but we'll acknowledge the attempt
		fmt.Println("⚠ Cannot decrement counter (Prometheus counters are monotonic)")

	case char == 'u' || char == 'U' || key == keyboard.KeyArrowUp:
		activeConnections.Inc()
		fmt.Println("✓ Active connections incremented")

	case char == 'd' || char == 'D' || key == keyboard.KeyArrowDown:
		activeConnections.Dec()
		fmt.Println("✓ Active connections decremented")

	case char == 'r' || char == 'R':
		// Simulate a response time (random-ish value based on counter)
		responseTimeMetric.Observe(0.1 + float64(char%10)*0.05)
		fmt.Println("✓ Response time recorded")

	case char == 's' || char == 'S':
		showMetrics()

	case char == 'q' || char == 'Q' || key == keyboard.KeyEsc:
		fmt.Println("\nGoodbye!")
		os.Exit(0)

	case char == 'h' || char == 'H' || char == '?':
		showHelp()

	default:
		if char != 0 {
			fmt.Printf("Unknown key: %c (press 'h' for help)\n", char)
		}
	}
}

func showHelp() {
	fmt.Println("\n=== Key Bindings ===")
	fmt.Println("  [+] or [=] - Increment request counter")
	fmt.Println("  [u] or [↑] - Increment active connections")
	fmt.Println("  [d] or [↓] - Decrement active connections")
	fmt.Println("  [r]        - Record a sample response time")
	fmt.Println("  [s]        - Show current metric values")
	fmt.Println("  [h] or [?] - Show this help")
	fmt.Println("  [q]        - Quit")
	fmt.Println()
}

func showMetrics() {
	fmt.Println("\n=== Current Metrics ===")
	fmt.Println("These are exported at http://localhost:8080/metrics")
	fmt.Println("Note: Counter values accumulate; use Prometheus to query rate()")
	fmt.Println()

	// Make a request to our own metrics endpoint
	resp, err := http.Get("http://localhost:8080/metrics")
	if err != nil {
		fmt.Printf("Error fetching metrics: %v\n", err)
		return
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		// Only show our custom metrics
		if strings.HasPrefix(line, "app_") && !strings.HasPrefix(line, "#") {
			fmt.Println(line)
		}
	}
	fmt.Println()
}
