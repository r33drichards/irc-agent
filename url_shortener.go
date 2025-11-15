package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
)

// URLShortener provides URL shortening functionality with HTTP serving
type URLShortener struct {
	mu       sync.RWMutex
	urlMap   map[string]string // maps short ID to original URL
	idLength int               // length of the short ID
	host     string            // the base URL for short links (e.g., "http://example.com:3000")
}

// NewURLShortener creates a new URL shortener instance
func NewURLShortener(host string) *URLShortener {
	return &URLShortener{
		urlMap:   make(map[string]string),
		idLength: 8,
		host:     host,
	}
}

// Shorten takes a URL (including signed URLs) and returns a short ID
func (us *URLShortener) Shorten(url string) string {
	us.mu.Lock()
	defer us.mu.Unlock()

	// Generate a short ID from the URL using SHA256
	hash := sha256.Sum256([]byte(url))
	shortID := hex.EncodeToString(hash[:])[:us.idLength]

	// Store the mapping
	us.urlMap[shortID] = url

	log.Printf("Shortened URL: %s -> %s", shortID, url)
	return shortID
}

// GetShortURL returns the full short URL for a given original URL
func (us *URLShortener) GetShortURL(url string) string {
	shortID := us.Shorten(url)
	return fmt.Sprintf("%s/%s", us.host, shortID)
}

// Serve starts the HTTP server on the specified port
func (us *URLShortener) Serve(port string) error {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Extract the ID from the path
		id := strings.TrimPrefix(r.URL.Path, "/")

		// Handle POST requests for creating short URLs
		if r.Method == http.MethodPost {
			if id != "" {
				http.Error(w, "POST only allowed at root path", http.StatusBadRequest)
				return
			}

			// Read the URL from request body
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read request body", http.StatusBadRequest)
				return
			}
			defer r.Body.Close()

			url := strings.TrimSpace(string(body))
			if url == "" {
				http.Error(w, "URL cannot be empty", http.StatusBadRequest)
				return
			}

			// Create short URL
			shortURL := us.GetShortURL(url)

			// Return the short URL
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, shortURL)
			log.Printf("Created short URL via POST: %s", shortURL)
			return
		}

		// Handle GET requests for redirects
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Handle root path
		if id == "" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "URL Shortener Service\n")
			fmt.Fprintf(w, "Usage:\n")
			fmt.Fprintf(w, "  GET  /<short-id> - Redirect to original URL\n")
			fmt.Fprintf(w, "  POST /           - Create short URL (send URL in body)\n")
			return
		}

		// Look up the original URL
		us.mu.RLock()
		originalURL, exists := us.urlMap[id]
		us.mu.RUnlock()

		if !exists {
			http.NotFound(w, r)
			log.Printf("Short ID not found: %s", id)
			return
		}

		// 301 redirect to the original URL
		log.Printf("Redirecting %s -> %s", id, originalURL)
		http.Redirect(w, r, originalURL, http.StatusMovedPermanently)
	})

	addr := ":" + port
	log.Printf("URL Shortener serving on %s", addr)
	return http.ListenAndServe(addr, nil)
}
