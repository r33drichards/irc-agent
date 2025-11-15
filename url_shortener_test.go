package main

import (
	"context"
	"testing"
)

func TestURLShortener(t *testing.T) {
	// Create a URL shortener with in-memory storage
	storage := NewInMemoryStorage()
	defer storage.Close()
	shortener := NewURLShortener("http://example.com:3000", storage)

	// Test URL
	testURL := "https://example.com/very/long/url/that/needs/to/be/shortened"

	// Shorten the URL
	shortID := shortener.Shorten(testURL)

	// Verify the short ID is 8 characters long
	if len(shortID) != 8 {
		t.Errorf("Expected short ID length of 8, got %d", len(shortID))
	}

	// Verify the URL is stored in the storage backend
	ctx := context.Background()
	storedURL, exists, err := storage.Get(ctx, shortID)

	if err != nil {
		t.Errorf("Error retrieving URL: %v", err)
	}

	if !exists {
		t.Errorf("Short ID %s not found in storage", shortID)
	}

	if storedURL != testURL {
		t.Errorf("Expected stored URL %s, got %s", testURL, storedURL)
	}

	// Test GetShortURL
	fullShortURL := shortener.GetShortURL(testURL)
	expectedURL := "http://example.com:3000/" + shortID
	if fullShortURL != expectedURL {
		t.Errorf("Expected full short URL %s, got %s", expectedURL, fullShortURL)
	}

	// Test that the same URL always generates the same short ID
	shortID2 := shortener.Shorten(testURL)
	if shortID != shortID2 {
		t.Errorf("Expected same short ID for same URL, got %s and %s", shortID, shortID2)
	}
}

func TestURLShortenerWithSignedURL(t *testing.T) {
	// Create a URL shortener with in-memory storage
	storage := NewInMemoryStorage()
	defer storage.Close()
	shortener := NewURLShortener("http://localhost:3000", storage)

	// Test with a signed URL (similar to S3 presigned URLs)
	signedURL := "https://robust-cicada.s3.us-west-2.amazonaws.com/code-results/1234567890-abcdef.txt?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=AKIAIOSFODNN7EXAMPLE%2F20231115%2Fus-west-2%2Fs3%2Faws4_request&X-Amz-Date=20231115T000000Z&X-Amz-Expires=86400&X-Amz-SignedHeaders=host&X-Amz-Signature=abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"

	// Shorten the signed URL
	shortID := shortener.Shorten(signedURL)

	// Verify the short ID is 8 characters long
	if len(shortID) != 8 {
		t.Errorf("Expected short ID length of 8, got %d", len(shortID))
	}

	// Verify the URL is stored in the storage backend
	ctx := context.Background()
	storedURL, exists, err := storage.Get(ctx, shortID)

	if err != nil {
		t.Errorf("Error retrieving URL: %v", err)
	}

	if !exists {
		t.Errorf("Short ID %s not found in storage", shortID)
	}

	if storedURL != signedURL {
		t.Errorf("Expected stored URL to match original signed URL")
	}

	// Get the full short URL
	fullShortURL := shortener.GetShortURL(signedURL)
	expectedURL := "http://localhost:3000/" + shortID
	if fullShortURL != expectedURL {
		t.Errorf("Expected full short URL %s, got %s", expectedURL, fullShortURL)
	}
}
