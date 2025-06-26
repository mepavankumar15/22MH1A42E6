package main

import (
	"log"
	"net/http"
	"time"
)

// CustomLogger is the logging middleware that wraps an HTTP handler
type CustomLogger struct {
	handler http.Handler
}

// ServeHTTP implements the http.Handler interface for the middleware
func (l *CustomLogger) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Call the next handler in the chain
	l.handler.ServeHTTP(w, r)

	// Log the request details after handling
	log.Printf(
		"[%s] %s %s %s %v",
		time.Now().Format("2006-01-02 15:04:05"),
		r.Method,
		r.URL.Path,
		r.RemoteAddr,
		time.Since(start),
	)
}

// WrapHandler applies the logging middleware to any http.Handler
func WrapHandler(handler http.Handler) *CustomLogger {
	return &CustomLogger{handler: handler}
}
