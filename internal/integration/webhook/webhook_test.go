// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"miniflux.app/v2/internal/config"
	"miniflux.app/v2/internal/model"
)

func allowPrivateNetworks(t *testing.T) {
	t.Helper()

	t.Setenv("INTEGRATION_ALLOW_PRIVATE_NETWORKS", "1")

	configParser := config.NewConfigParser()
	parsedOptions, err := configParser.ParseEnvironmentVariables()
	if err != nil {
		t.Fatalf("Unable to configure test options: %v", err)
	}

	previousOptions := config.Opts
	config.Opts = parsedOptions
	t.Cleanup(func() {
		config.Opts = previousOptions
	})
}

func digestTestEntries() model.Entries {
	return model.Entries{
		{ID: 1, FeedID: 10, Title: "Entry A1", URL: "https://example.org/a1", Date: time.Date(2026, 9, 20, 1, 0, 0, 0, time.UTC), Feed: &model.Feed{ID: 10, Title: "Feed A"}},
		{ID: 2, FeedID: 10, Title: "Entry A2", URL: "https://example.org/a2", Date: time.Date(2026, 9, 20, 2, 0, 0, 0, time.UTC), Feed: &model.Feed{ID: 10, Title: "Feed A"}},
		{ID: 3, FeedID: 20, Title: "Entry B1", URL: "https://example.org/b1", Date: time.Date(2026, 9, 20, 3, 0, 0, 0, time.UTC), Feed: &model.Feed{ID: 20, Title: "Feed B"}},
		// Duplicate of entry ID 2: must not appear twice in the digest.
		{ID: 2, FeedID: 10, Title: "Entry A2", URL: "https://example.org/a2", Date: time.Date(2026, 9, 20, 2, 0, 0, 0, time.UTC), Feed: &model.Feed{ID: 10, Title: "Feed A"}},
	}
}

func TestNewDigestEventGroupsByFeedAndDeduplicates(t *testing.T) {
	event := NewDigestEvent(digestTestEntries())

	if event.EventType != DigestEventType {
		t.Fatalf(`Unexpected event type, got %q`, event.EventType)
	}
	if len(event.Feeds) != 2 {
		t.Fatalf(`Expected 2 feeds in digest, got %d`, len(event.Feeds))
	}

	feedA := event.Feeds[0]
	if feedA.Feed.Title != "Feed A" {
		t.Errorf(`Unexpected feed title, got %q`, feedA.Feed.Title)
	}
	if feedA.EntryCount != 2 || len(feedA.Entries) != 2 {
		t.Fatalf(`Expected 2 entries for feed A, got count=%d entries=%d`, feedA.EntryCount, len(feedA.Entries))
	}
	if feedA.Entries[0].Title != "Entry A1" || feedA.Entries[0].URL != "https://example.org/a1" {
		t.Errorf(`Unexpected first entry: %+v`, feedA.Entries[0])
	}

	feedB := event.Feeds[1]
	if feedB.Feed.Title != "Feed B" || feedB.EntryCount != 1 {
		t.Errorf(`Unexpected feed B digest: %+v`, feedB)
	}
}

func TestSendDigestWebhookEvent(t *testing.T) {
	allowPrivateNetworks(t)

	var receivedEventType string
	var receivedPayload WebhookDigestEvent

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedEventType = r.Header.Get("X-Miniflux-Event-Type")
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

	client := NewClient(server.URL, "secret")
	if err := client.SendDigestWebhookEvent(digestTestEntries()); err != nil {
		t.Fatalf(`Unable to send digest webhook event: %v`, err)
	}

	if receivedEventType != DigestEventType {
		t.Fatalf(`Unexpected event type header, got %q`, receivedEventType)
	}
	if receivedPayload.EventType != DigestEventType {
		t.Fatalf(`Unexpected payload event type, got %q`, receivedPayload.EventType)
	}
	if len(receivedPayload.Feeds) != 2 {
		t.Fatalf(`Expected 2 feeds in payload, got %d`, len(receivedPayload.Feeds))
	}

	totalEntries := 0
	for _, feed := range receivedPayload.Feeds {
		if feed.Feed.Title == "" {
			t.Error(`Feed title should not be empty in digest payload`)
		}
		if feed.EntryCount != len(feed.Entries) {
			t.Errorf(`Entry count %d does not match entries length %d`, feed.EntryCount, len(feed.Entries))
		}
		for _, entry := range feed.Entries {
			if entry.Title == "" || entry.URL == "" {
				t.Errorf(`Digest entry must have a title and a URL: %+v`, entry)
			}
		}
		totalEntries += feed.EntryCount
	}
	if totalEntries != 3 {
		t.Fatalf(`Expected 3 deduplicated entries in digest, got %d`, totalEntries)
	}
}

func TestSendDigestWebhookEventWithNoEntries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error(`The webhook endpoint should not be called when there is nothing to send`)
	}))
	defer server.Close()

	client := NewClient(server.URL, "secret")
	if err := client.SendDigestWebhookEvent(nil); err != nil {
		t.Fatalf(`Unexpected error: %v`, err)
	}
}
