// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage // import "miniflux.app/v2/internal/storage"

import (
	"fmt"
	"time"

	"miniflux.app/v2/internal/model"
)

// DueDigestIntegrations returns the integrations of users with an enabled
// digest whose interval has elapsed. The digest is delivered through the
// webhook integration, so rows without a configured webhook are skipped.
func (s *Storage) DueDigestIntegrations() ([]*model.Integration, error) {
	query := `
		SELECT
			user_id,
			webhook_url,
			webhook_secret,
			digest_interval_hours
		FROM
			integrations
		WHERE
			digest_enabled='t'
			AND digest_interval_hours > 0
			AND webhook_enabled='t'
			AND webhook_url <> ''
			AND (
				digest_last_sent_at IS NULL
				OR digest_last_sent_at + digest_interval_hours * interval '1 hour' <= now()
			)
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("store: unable to fetch due digest integrations: %w", err)
	}
	defer rows.Close()

	var integrations []*model.Integration
	for rows.Next() {
		var integration model.Integration
		if err := rows.Scan(
			&integration.UserID,
			&integration.WebhookURL,
			&integration.WebhookSecret,
			&integration.DigestIntervalHours,
		); err != nil {
			return nil, fmt.Errorf("store: unable to scan digest integration row: %w", err)
		}
		integration.DigestEnabled = true
		integration.WebhookEnabled = true
		integrations = append(integrations, &integration)
	}

	return integrations, rows.Err()
}

// UnreadDigestEntries returns the unread entries of a user along with their
// feed title, ordered by feed then publication date. Entry statuses are never
// modified by this query.
func (s *Storage) UnreadDigestEntries(userID int64) (model.Entries, error) {
	query := `
		SELECT
			e.id,
			e.feed_id,
			e.title,
			e.url,
			e.published_at,
			f.title
		FROM
			entries e
		JOIN
			feeds f ON f.id = e.feed_id
		WHERE
			e.user_id=$1 AND e.status=$2
		ORDER BY
			f.title, e.published_at
	`

	rows, err := s.db.Query(query, userID, model.EntryStatusUnread)
	if err != nil {
		return nil, fmt.Errorf("store: unable to fetch unread digest entries: %w", err)
	}
	defer rows.Close()

	var entries model.Entries
	for rows.Next() {
		var entry model.Entry
		entry.Feed = &model.Feed{}
		if err := rows.Scan(
			&entry.ID,
			&entry.FeedID,
			&entry.Title,
			&entry.URL,
			&entry.Date,
			&entry.Feed.Title,
		); err != nil {
			return nil, fmt.Errorf("store: unable to scan unread digest entry: %w", err)
		}
		entry.UserID = userID
		entry.Status = model.EntryStatusUnread
		entries = append(entries, &entry)
	}

	return entries, rows.Err()
}

// UpdateDigestLastSentAt records when the digest of a user was last handled.
func (s *Storage) UpdateDigestLastSentAt(userID int64, sentAt time.Time) error {
	query := `UPDATE integrations SET digest_last_sent_at=$1 WHERE user_id=$2`
	if _, err := s.db.Exec(query, sentAt, userID); err != nil {
		return fmt.Errorf("store: unable to update digest last sent date: %w", err)
	}
	return nil
}
