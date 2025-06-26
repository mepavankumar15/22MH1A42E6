package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/speps/go-hashids"
)

// CustomLogger is the logging middleware from Pre-Test Setup
type CustomLogger struct {
	handler http.Handler
}

func (l *CustomLogger) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	l.handler.ServeHTTP(w, r)
	log.Printf("%s %s %v", r.Method, r.URL.Path, time.Since(start))
}

// In-memory storage
var (
	urlStore  = make(map[string]ShortURL)
	analytics = make(map[string][]Click)
	storeLock sync.RWMutex
)

// Models
type ShortURL struct {
	ShortCode   string    `json:"shortCode"`
	OriginalURL string    `json:"originalUrl"`
	CreatedAt   time.Time `json:"createdAt"`
	ExpiresAt   time.Time `json:"expiresAt"`
	IsActive    bool      `json:"isActive"`
}

type ShortURLRequest struct {
	URL       string `json:"url"`
	Validity  int    `json:"validity"`
	Shortcode string `json:"shortcode"`
}

type ShortURLResponse struct {
	ShortLink string `json:"shortLink"`
	Expiry    string `json:"expiry"`
}

type URLStats struct {
	OriginalURL  string    `json:"originalUrl"`
	CreatedAt    time.Time `json:"createdAt"`
	ExpiresAt    time.Time `json:"expiresAt"`
	TotalClicks  int       `json:"totalClicks"`
	ClickDetails []Click   `json:"clickDetails"`
}

type Click struct {
	Timestamp time.Time `json:"timestamp"`
	Referrer  string    `json:"referrer"`
	UserAgent string    `json:"userAgent"`
	IPAddress string    `json:"ipAddress"`
}

// Handlers
func createShortURL(w http.ResponseWriter, r *http.Request) {
	var req ShortURLRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Validate URL
	if !strings.HasPrefix(req.URL, "http://") && !strings.HasPrefix(req.URL, "https://") {
		http.Error(w, `{"error": "URL must start with http:// or https://"}`, http.StatusBadRequest)
		return
	}

	// Set default validity if not provided
	if req.Validity == 0 {
		req.Validity = 30
	}

	expiresAt := time.Now().Add(time.Duration(req.Validity) * time.Minute)

	var shortCode string
	if req.Shortcode != "" {
		// Check if custom shortcode is available
		storeLock.RLock()
		_, exists := urlStore[req.Shortcode]
		storeLock.RUnlock()

		if exists {
			http.Error(w, `{"error": "Shortcode already in use"}`, http.StatusConflict)
			return
		}
		shortCode = req.Shortcode
	} else {
		// Generate unique shortcode
		hd := hashids.NewData()
		hd.Salt = "url-shortener-salt"
		hd.MinLength = 5
		h, _ := hashids.NewWithData(hd)
		shortCode, _ = h.Encode([]int{int(time.Now().Unix())})
	}

	// Store in memory
	newURL := ShortURL{
		ShortCode:   shortCode,
		OriginalURL: req.URL,
		CreatedAt:   time.Now(),
		ExpiresAt:   expiresAt,
		IsActive:    true,
	}

	storeLock.Lock()
	urlStore[shortCode] = newURL
	analytics[shortCode] = []Click{}
	storeLock.Unlock()

	host := r.Host
	if host == "" {
		host = "localhost:8080"
	}

	response := ShortURLResponse{
		ShortLink: fmt.Sprintf("http://%s/%s", host, shortCode),
		Expiry:    expiresAt.Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

func redirectShortURL(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	shortCode := vars["shortcode"]

	storeLock.RLock()
	url, exists := urlStore[shortCode]
	storeLock.RUnlock()

	if !exists || !url.IsActive {
		http.Error(w, `{"error": "Short URL not found"}`, http.StatusNotFound)
		return
	}

	if time.Now().After(url.ExpiresAt) {
		http.Error(w, `{"error": "Short URL has expired"}`, http.StatusGone)
		return
	}

	// Record analytics
	click := Click{
		Timestamp: time.Now(),
		Referrer:  r.Referer(),
		UserAgent: r.UserAgent(),
		IPAddress: strings.Split(r.RemoteAddr, ":")[0],
	}

	storeLock.Lock()
	analytics[shortCode] = append(analytics[shortCode], click)
	storeLock.Unlock()

	http.Redirect(w, r, url.OriginalURL, http.StatusFound)
}

func getURLStats(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	shortCode := vars["shortcode"]

	storeLock.RLock()
	url, exists := urlStore[shortCode]
	clicks := analytics[shortCode]
	storeLock.RUnlock()

	if !exists {
		http.Error(w, `{"error": "Short URL not found"}`, http.StatusNotFound)
		return
	}

	stats := URLStats{
		OriginalURL:  url.OriginalURL,
		CreatedAt:    url.CreatedAt,
		ExpiresAt:    url.ExpiresAt,
		TotalClicks:  len(clicks),
		ClickDetails: clicks,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func main() {
	r := mux.NewRouter()

	// API routes
	r.HandleFunc("/shorturls", createShortURL).Methods("POST")
	r.HandleFunc("/{shortcode}", redirectShortURL).Methods("GET")
	r.HandleFunc("/shorturls/{shortcode}", getURLStats).Methods("GET")

	// Wrap with logging middleware
	loggedRouter := &CustomLogger{handler: r}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, loggedRouter))
}
