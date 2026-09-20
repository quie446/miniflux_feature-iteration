// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package digest // import "miniflux.app/v2/internal/digest"

import (
	"log/slog"
	"time"

	"miniflux.app/v2/internal/integration/webhook"
	"miniflux.app/v2/internal/model"
	"miniflux.app/v2/internal/storage"
)

// SendPendingDigests sends a digest of unread entries over the user webhook
// for every user whose digest interval has elapsed. Users with the digest
// disabled are never selected, so the feature stays completely silent when
// turned off.
func SendPendingDigests(store *storage.Storage) {
	users, err := store.UsersWithPendingDigest()
	if err != nil {
		slog.Error("Unable to fetch users with pending digest", slog.Any("error", err))
		return
	}

	for _, user := range users {
		if err := sendUserDigest(store, user); err != nil {
			slog.Error("Unable to send digest",
				slog.Int64("user_id", user.ID),
				slog.Any("error", err),
			)
		}
	}
}

func sendUserDigest(store *storage.Storage, user *model.User) error {
	userIntegrations, err := store.Integration(user.ID)
	if err != nil {
		return err
	}

	entries, err := store.UnreadDigestEntries(user.ID)
	if err != nil {
		return err
	}

	sentAt := time.Now()

	if len(entries) == 0 || !userIntegrations.WebhookEnabled || userIntegrations.WebhookURL == "" {
		// Nothing to deliver: still move the schedule forward so the next
		// digest covers a full interval.
		return store.MarkDigestEntriesSent(user.ID, nil, sentAt)
	}

	slog.Debug("Sending digest to Webhook",
		slog.Int64("user_id", user.ID),
		slog.Int("nb_entries", len(entries)),
		slog.String("webhook_url", userIntegrations.WebhookURL),
	)

	webhookClient := webhook.NewClient(userIntegrations.WebhookURL, userIntegrations.WebhookSecret)
	if err := webhookClient.SendDigestWebhookEvent(user.ID, entries); err != nil {
		return err
	}

	entryIDs := make([]int64, 0, len(entries))
	for _, entry := range entries {
		entryIDs = append(entryIDs, entry.ID)
	}

	// The entries are only recorded as "sent in a digest" after a successful
	// delivery; their read/unread status is left untouched.
	return store.MarkDigestEntriesSent(user.ID, entryIDs, sentAt)
}
