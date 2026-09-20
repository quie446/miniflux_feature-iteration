// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"miniflux.app/v2/internal/config"
	"miniflux.app/v2/internal/model"
)

func configureIntegrationAllowPrivateNetworksOption(t *testing.T) {
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

func TestSendDigestWebhookEvent(t *testing.T) {
	configureIntegrationAllowPrivateNetworksOption(t)

	var (
		receivedEventType string
		receivedBody      []byte
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedEventType = r.Header.Get("X-Miniflux-Event-Type")
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("unable to read request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	entries := model.Entries{
		{ID: 1, FeedID: 10, Title: "Entry A", URL: "https://example.org/a", Feed: &model.Feed{ID: 10, Title: "Feed One"}},
		{ID: 2, FeedID: 10, Title: "Entry B", URL: "https://example.org/b", Feed: &model.Feed{ID: 10, Title: "Feed One"}},
		{ID: 3, FeedID: 20, Title: "Entry C", URL: "https://example.org/c", Feed: &model.Feed{ID: 20, Title: "Feed Two"}},
		// Duplicate of entry #1: it must not appear twice in the digest.
		{ID: 1, FeedID: 10, Title: "Entry A", URL: "https://example.org/a", Feed: &model.Feed{ID: 10, Title: "Feed One"}},
	}

	client := NewClient(server.URL, "secret")
	if err := client.SendDigestWebhookEvent(42, entries); err != nil {
		t.Fatalf("unable to send digest webhook event: %v", err)
	}

	if receivedEventType != DigestEventType {
		t.Fatalf("unexpected event type header: got %q, want %q", receivedEventType, DigestEventType)
	}

	var event WebhookDigestEvent
	if err := json.Unmarshal(receivedBody, &event); err != nil {
		t.Fatalf("unable to decode digest payload: %v", err)
	}

	if event.EventType != DigestEventType {
		t.Errorf("unexpected event type: got %q, want %q", event.EventType, DigestEventType)
	}

	if event.UserID != 42 {
		t.Errorf("unexpected user ID: got %d, want 42", event.UserID)
	}

	if event.TotalUnread != 3 {
		t.Errorf("unexpected total unread count: got %d, want 3", event.TotalUnread)
	}

	if len(event.Feeds) != 2 {
		t.Fatalf("unexpected number of feeds: got %d, want 2", len(event.Feeds))
	}

	firstFeed := event.Feeds[0]
	if firstFeed.FeedID != 10 || firstFeed.FeedTitle != "Feed One" {
		t.Errorf("unexpected first feed: got %+v", firstFeed)
	}

	if firstFeed.UnreadCount != 2 || len(firstFeed.Entries) != 2 {
		t.Fatalf("unexpected unread count for first feed: got %d entries, want 2", len(firstFeed.Entries))
	}

	if firstFeed.Entries[0].Title != "Entry A" || firstFeed.Entries[0].URL != "https://example.org/a" {
		t.Errorf("unexpected first entry: got %+v", firstFeed.Entries[0])
	}

	secondFeed := event.Feeds[1]
	if secondFeed.FeedID != 20 || secondFeed.FeedTitle != "Feed Two" || secondFeed.UnreadCount != 1 {
		t.Errorf("unexpected second feed: got %+v", secondFeed)
	}
}

func TestSendDigestWebhookEventWithNoEntries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("the webhook endpoint must not be called when there is nothing to send")
	}))
	defer server.Close()

	client := NewClient(server.URL, "secret")
	if err := client.SendDigestWebhookEvent(42, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
