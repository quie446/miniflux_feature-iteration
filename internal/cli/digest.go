// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cli // import "miniflux.app/v2/internal/cli"

import (
	"log/slog"
	"time"

	"miniflux.app/v2/internal/integration/webhook"
	"miniflux.app/v2/internal/storage"
)

// digestPollingFrequency is how often the scheduler checks for due digests.
const digestPollingFrequency = time.Minute

func digestScheduler(store *storage.Storage, frequency time.Duration) {
	for range time.Tick(frequency) {
		runDigestTasks(store)
	}
}

// runDigestTasks sends one aggregated digest of unread entries to each user
// whose digest interval has elapsed. Entry statuses are left untouched.
func runDigestTasks(store *storage.Storage) {
	integrations, err := store.DueDigestIntegrations()
	if err != nil {
		slog.Error("Unable to fetch due digest integrations", slog.Any("error", err))
		return
	}

	for _, integration := range integrations {
		entries, err := store.UnreadDigestEntries(integration.UserID)
		if err != nil {
			slog.Error("Unable to fetch unread entries for digest",
				slog.Int64("user_id", integration.UserID),
				slog.Any("error", err),
			)
			continue
		}

		if len(entries) > 0 {
			slog.Debug("Sending digest to Webhook",
				slog.Int64("user_id", integration.UserID),
				slog.Int("nb_entries", len(entries)),
			)

			webhookClient := webhook.NewClient(integration.WebhookURL, integration.WebhookSecret)
			if err := webhookClient.SendDigestWebhookEvent(entries); err != nil {
				slog.Warn("Unable to send digest to Webhook",
					slog.Int64("user_id", integration.UserID),
					slog.Int("nb_entries", len(entries)),
					slog.Any("error", err),
				)
				continue
			}
		}

		if err := store.UpdateDigestLastSentAt(integration.UserID, time.Now()); err != nil {
			slog.Error("Unable to update digest last sent date",
				slog.Int64("user_id", integration.UserID),
				slog.Any("error", err),
			)
		}
	}
}
