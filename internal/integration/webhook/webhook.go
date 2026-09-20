// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package webhook // import "miniflux.app/v2/internal/integration/webhook"

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"miniflux.app/v2/internal/crypto"
	"miniflux.app/v2/internal/http/client"
	"miniflux.app/v2/internal/model"
)

const (
	NewEntriesEventType = "new_entries"
	SaveEntryEventType  = "save_entry"
	DigestEventType     = "digest"
)

type Client struct {
	webhookURL    string
	webhookSecret string
}

func NewClient(webhookURL, webhookSecret string) *Client {
	return &Client{webhookURL, webhookSecret}
}

func (c *Client) SendSaveEntryWebhookEvent(entry *model.Entry) error {
	return c.makeRequest(SaveEntryEventType, &WebhookSaveEntryEvent{
		EventType: SaveEntryEventType,
		Entry: &WebhookEntry{
			ID:          entry.ID,
			UserID:      entry.UserID,
			FeedID:      entry.FeedID,
			Status:      entry.Status,
			Hash:        entry.Hash,
			Title:       entry.Title,
			URL:         entry.URL,
			CommentsURL: entry.CommentsURL,
			Date:        entry.Date,
			CreatedAt:   entry.CreatedAt,
			ChangedAt:   entry.ChangedAt,
			Content:     entry.Content,
			Author:      entry.Author,
			ShareCode:   entry.ShareCode,
			Starred:     entry.Starred,
			ReadingTime: entry.ReadingTime,
			Enclosures:  entry.Enclosures,
			Tags:        entry.Tags,
			Feed: &WebhookFeed{
				ID:         entry.Feed.ID,
				UserID:     entry.Feed.UserID,
				CategoryID: entry.Feed.Category.ID,
				Category:   &WebhookCategory{ID: entry.Feed.Category.ID, Title: entry.Feed.Category.Title},
				FeedURL:    entry.Feed.FeedURL,
				SiteURL:    entry.Feed.SiteURL,
				Title:      entry.Feed.Title,
				CheckedAt:  entry.Feed.CheckedAt,
			},
		},
	})
}

func (c *Client) SendNewEntriesWebhookEvent(feed *model.Feed, entries model.Entries) error {
	if len(entries) == 0 {
		return nil
	}

	webhookEntries := make([]*WebhookEntry, 0, len(entries))
	for _, entry := range entries {
		webhookEntries = append(webhookEntries, &WebhookEntry{
			ID:          entry.ID,
			UserID:      entry.UserID,
			FeedID:      entry.FeedID,
			Status:      entry.Status,
			Hash:        entry.Hash,
			Title:       entry.Title,
			URL:         entry.URL,
			CommentsURL: entry.CommentsURL,
			Date:        entry.Date,
			CreatedAt:   entry.CreatedAt,
			ChangedAt:   entry.ChangedAt,
			Content:     entry.Content,
			Author:      entry.Author,
			ShareCode:   entry.ShareCode,
			Starred:     entry.Starred,
			ReadingTime: entry.ReadingTime,
			Enclosures:  entry.Enclosures,
			Tags:        entry.Tags,
		})
	}
	return c.makeRequest(NewEntriesEventType, &WebhookNewEntriesEvent{
		EventType: NewEntriesEventType,
		Feed: &WebhookFeed{
			ID:         feed.ID,
			UserID:     feed.UserID,
			CategoryID: feed.Category.ID,
			Category:   &WebhookCategory{ID: feed.Category.ID, Title: feed.Category.Title},
			FeedURL:    feed.FeedURL,
			SiteURL:    feed.SiteURL,
			Title:      feed.Title,
			CheckedAt:  feed.CheckedAt,
		},
		Entries: webhookEntries,
	})
}

// SendDigestWebhookEvent sends a digest of unread entries grouped by feed.
func (c *Client) SendDigestWebhookEvent(entries model.Entries) error {
	event := NewDigestEvent(entries)
	if len(event.Feeds) == 0 {
		return nil
	}
	return c.makeRequest(DigestEventType, event)
}

// NewDigestEvent builds a digest event from a list of entries. Entries are
// grouped by feed and deduplicated by entry ID, so the same entry never
// appears twice in a single digest.
func NewDigestEvent(entries model.Entries) *WebhookDigestEvent {
	event := &WebhookDigestEvent{EventType: DigestEventType}
	seen := make(map[int64]bool, len(entries))
	feedIndex := make(map[int64]int)

	for _, entry := range entries {
		if entry == nil || seen[entry.ID] {
			continue
		}
		seen[entry.ID] = true

		index, found := feedIndex[entry.FeedID]
		if !found {
			digestFeed := &WebhookDigestFeed{
				Feed: &WebhookFeed{
					ID: entry.FeedID,
				},
			}
			if entry.Feed != nil {
				digestFeed.Feed.UserID = entry.Feed.UserID
				digestFeed.Feed.FeedURL = entry.Feed.FeedURL
				digestFeed.Feed.SiteURL = entry.Feed.SiteURL
				digestFeed.Feed.Title = entry.Feed.Title
			}
			event.Feeds = append(event.Feeds, digestFeed)
			index = len(event.Feeds) - 1
			feedIndex[entry.FeedID] = index
		}

		digestFeed := event.Feeds[index]
		digestFeed.Entries = append(digestFeed.Entries, &WebhookDigestEntry{
			ID:    entry.ID,
			Title: entry.Title,
			URL:   entry.URL,
			Date:  entry.Date,
		})
		digestFeed.EntryCount++
	}

	return event
}

func (c *Client) makeRequest(eventType string, payload any) error {
	if c.webhookURL == "" {
		return errors.New(`webhook: missing webhook URL`)
	}

	requestBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("webhook: unable to encode request body: %v", err)
	}

	response, err := client.NewRequestBuilder(c.webhookURL).
		WithMethod(http.MethodPost).
		WithJSONBody(requestBody).
		WithHeader("X-Miniflux-Signature", crypto.GenerateSHA256Hmac(c.webhookSecret, requestBody)).
		WithHeader("X-Miniflux-Event-Type", eventType).
		Do()
	if err != nil {
		return fmt.Errorf("webhook: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode >= 400 {
		return fmt.Errorf("webhook: incorrect response status code %d for url %s", response.StatusCode, c.webhookURL)
	}

	return nil
}

type WebhookFeed struct {
	ID         int64            `json:"id"`
	UserID     int64            `json:"user_id"`
	CategoryID int64            `json:"category_id"`
	Category   *WebhookCategory `json:"category,omitempty"`
	FeedURL    string           `json:"feed_url"`
	SiteURL    string           `json:"site_url"`
	Title      string           `json:"title"`
	CheckedAt  time.Time        `json:"checked_at"`
}

type WebhookCategory struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

type WebhookEntry struct {
	ID          int64               `json:"id"`
	UserID      int64               `json:"user_id"`
	FeedID      int64               `json:"feed_id"`
	Status      string              `json:"status"`
	Hash        string              `json:"hash"`
	Title       string              `json:"title"`
	URL         string              `json:"url"`
	CommentsURL string              `json:"comments_url"`
	Date        time.Time           `json:"published_at"`
	CreatedAt   time.Time           `json:"created_at"`
	ChangedAt   time.Time           `json:"changed_at"`
	Content     string              `json:"content"`
	Author      string              `json:"author"`
	ShareCode   string              `json:"share_code"`
	Starred     bool                `json:"starred"`
	ReadingTime int                 `json:"reading_time"`
	Enclosures  model.EnclosureList `json:"enclosures"`
	Tags        []string            `json:"tags"`
	Feed        *WebhookFeed        `json:"feed,omitempty"`
}

type WebhookNewEntriesEvent struct {
	EventType string          `json:"event_type"`
	Feed      *WebhookFeed    `json:"feed"`
	Entries   []*WebhookEntry `json:"entries"`
}

type WebhookSaveEntryEvent struct {
	EventType string        `json:"event_type"`
	Entry     *WebhookEntry `json:"entry"`
}

type WebhookDigestEntry struct {
	ID    int64     `json:"id"`
	Title string    `json:"title"`
	URL   string    `json:"url"`
	Date  time.Time `json:"published_at"`
}

type WebhookDigestFeed struct {
	Feed       *WebhookFeed          `json:"feed"`
	EntryCount int                   `json:"entry_count"`
	Entries    []*WebhookDigestEntry `json:"entries"`
}

type WebhookDigestEvent struct {
	EventType string               `json:"event_type"`
	Feeds     []*WebhookDigestFeed `json:"feeds"`
}
