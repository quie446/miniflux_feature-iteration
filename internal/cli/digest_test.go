// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"miniflux.app/v2/internal/config"
	"miniflux.app/v2/internal/database"
	"miniflux.app/v2/internal/integration/webhook"
	"miniflux.app/v2/internal/storage"
)

// TestRunDigestTasksEndToEnd exercises the full digest path against a real
// PostgreSQL database: enable the digest, create unread entries, run the
// digest task once, and assert the webhook receives the aggregated payload.
//
// Usage:
//
//	DIGEST_TEST_DATABASE_URL="postgres://user:pass@localhost:5432/digest_test?sslmode=disable" go test ./internal/cli/ -run TestRunDigestTasksEndToEnd -v
func TestRunDigestTasksEndToEnd(t *testing.T) {
	dsn := os.Getenv("DIGEST_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("DIGEST_TEST_DATABASE_URL is not set")
	}

	// The webhook mock server listens on 127.0.0.1.
	t.Setenv("INTEGRATION_ALLOW_PRIVATE_NETWORKS", "1")
	configParser := config.NewConfigParser()
	parsedOptions, err := configParser.ParseEnvironmentVariables()
	if err != nil {
		t.Fatalf("Unable to parse test options: %v", err)
	}
	previousOptions := config.Opts
	config.Opts = parsedOptions
	t.Cleanup(func() {
		config.Opts = previousOptions
	})

	db, err := database.NewConnectionPool(dsn, 1, 1, time.Minute)
	if err != nil {
		t.Fatalf("Unable to connect to the database: %v", err)
	}
	defer db.Close()

	if err := database.Migrate(db); err != nil {
		t.Fatalf("Unable to run database migrations: %v", err)
	}

	store := storage.NewStorage(db)

	requestCount := 0
	var receivedPayload webhook.WebhookDigestEvent
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if eventType := r.Header.Get("X-Miniflux-Event-Type"); eventType != webhook.DigestEventType {
			t.Errorf(`Unexpected event type header, got %q`, eventType)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf(`Unable to read request body: %v`, err)
		}
		if err := json.Unmarshal(body, &receivedPayload); err != nil {
			t.Errorf(`Unable to decode digest payload: %v`, err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var userID int64
	if err := db.QueryRow(`INSERT INTO users (username) VALUES ('digest_test_user') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("Unable to create test user: %v", err)
	}

	var categoryID int64
	if err := db.QueryRow(`INSERT INTO categories (user_id, title) VALUES ($1, 'All') RETURNING id`, userID).Scan(&categoryID); err != nil {
		t.Fatalf("Unable to create test category: %v", err)
	}

	feedIDs := make([]int64, 2)
	for i, title := range []string{"Digest Feed A", "Digest Feed B"} {
		if err := db.QueryRow(
			`INSERT INTO feeds (user_id, category_id, title, feed_url, site_url) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			userID, categoryID, title, "https://example.org/feed_"+title, "https://example.org/",
		).Scan(&feedIDs[i]); err != nil {
			t.Fatalf("Unable to create test feed: %v", err)
		}
	}

	entries := []struct {
		feedID int64
		hash   string
		title  string
		url    string
	}{
		{feedIDs[0], "hash_a1", "Entry A1", "https://example.org/a1"},
		{feedIDs[0], "hash_a2", "Entry A2", "https://example.org/a2"},
		{feedIDs[1], "hash_b1", "Entry B1", "https://example.org/b1"},
	}
	for _, entry := range entries {
		if _, err := db.Exec(
			`INSERT INTO entries (user_id, feed_id, hash, published_at, created_at, changed_at, title, url) VALUES ($1, $2, $3, now(), now(), now(), $4, $5)`,
			userID, entry.feedID, entry.hash, entry.title, entry.url,
		); err != nil {
			t.Fatalf("Unable to create test entry: %v", err)
		}
	}

	if _, err := db.Exec(
		`INSERT INTO integrations (user_id, webhook_enabled, webhook_url, digest_enabled, digest_interval_hours) VALUES ($1, true, $2, true, 1)`,
		userID, server.URL,
	); err != nil {
		t.Fatalf("Unable to create test integration: %v", err)
	}

	runDigestTasks(store)

	if requestCount != 1 {
		t.Fatalf("Expected exactly one webhook request, got %d", requestCount)
	}
	if receivedPayload.EventType != webhook.DigestEventType {
		t.Fatalf(`Unexpected payload event type, got %q`, receivedPayload.EventType)
	}
	if len(receivedPayload.Feeds) != 2 {
		t.Fatalf("Expected 2 feeds in digest payload, got %d", len(receivedPayload.Feeds))
	}

	totalEntries := 0
	for _, feed := range receivedPayload.Feeds {
		if feed.Feed.Title == "" {
			t.Error("Feed title should not be empty in digest payload")
		}
		if feed.EntryCount != len(feed.Entries) {
			t.Errorf("Entry count %d does not match entries length %d", feed.EntryCount, len(feed.Entries))
		}
		for _, entry := range feed.Entries {
			if entry.Title == "" || entry.URL == "" {
				t.Errorf("Digest entry must have a title and a URL: %+v", entry)
			}
		}
		totalEntries += feed.EntryCount
	}
	if totalEntries != 3 {
		t.Fatalf("Expected 3 entries in digest payload, got %d", totalEntries)
	}

	// The digest must never change entry statuses.
	var unreadCount int
	if err := db.QueryRow(`SELECT count(*) FROM entries WHERE user_id=$1 AND status='unread'`, userID).Scan(&unreadCount); err != nil {
		t.Fatalf("Unable to count unread entries: %v", err)
	}
	if unreadCount != 3 {
		t.Fatalf("Expected 3 unread entries after digest, got %d", unreadCount)
	}

	// The digest timestamp must be recorded to respect the interval.
	var lastSentAt *time.Time
	if err := db.QueryRow(`SELECT digest_last_sent_at FROM integrations WHERE user_id=$1`, userID).Scan(&lastSentAt); err != nil {
		t.Fatalf("Unable to fetch digest_last_sent_at: %v", err)
	}
	if lastSentAt == nil {
		t.Fatal("digest_last_sent_at should be set after a successful digest")
	}

	// The interval has not elapsed yet: a second run must stay silent.
	runDigestTasks(store)
	if requestCount != 1 {
		t.Fatalf("Expected no additional webhook request before the interval elapses, got %d", requestCount)
	}
}
