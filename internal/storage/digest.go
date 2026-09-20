// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage // import "miniflux.app/v2/internal/storage"

import (
	"fmt"
	"time"

	"miniflux.app/v2/internal/model"
)

// UsersWithPendingDigest returns the users who enabled the digest and whose
// digest interval has elapsed since the last digest was sent.
func (s *Storage) UsersWithPendingDigest() (model.Users, error) {
	query := `
		SELECT
			id,
			digest_interval_hours
		FROM
			users
		WHERE
			digest_enabled
			AND digest_interval_hours > 0
			AND (
				last_digest_sent_at IS NULL
				OR last_digest_sent_at + make_interval(hours => digest_interval_hours) <= now()
			)
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf(`store: unable to fetch users with pending digest: %v`, err)
	}
	defer rows.Close()

	var users model.Users
	for rows.Next() {
		var user model.User
		if err := rows.Scan(&user.ID, &user.DigestIntervalHours); err != nil {
			return nil, fmt.Errorf(`store: unable to fetch user with pending digest row: %v`, err)
		}
		users = append(users, &user)
	}

	return users, nil
}

// UnreadDigestEntries returns the unread entries of the given user that have
// not been sent in a previous digest, ordered by feed then publication date.
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
		INNER JOIN
			feeds f ON f.id = e.feed_id
		LEFT JOIN
			digest_entries de ON de.user_id = e.user_id AND de.entry_id = e.id
		WHERE
			e.user_id = $1
			AND e.status = $2
			AND de.entry_id IS NULL
		ORDER BY
			f.title ASC,
			e.published_at ASC
	`
	rows, err := s.db.Query(query, userID, model.EntryStatusUnread)
	if err != nil {
		return nil, fmt.Errorf(`store: unable to fetch digest entries for user #%d: %v`, userID, err)
	}
	defer rows.Close()

	var entries model.Entries
	for rows.Next() {
		var entry model.Entry
		entry.UserID = userID
		entry.Status = model.EntryStatusUnread
		entry.Feed = &model.Feed{}
		if err := rows.Scan(&entry.ID, &entry.FeedID, &entry.Title, &entry.URL, &entry.Date, &entry.Feed.Title); err != nil {
			return nil, fmt.Errorf(`store: unable to fetch digest entry row: %v`, err)
		}
		entry.Feed.ID = entry.FeedID
		entry.Feed.UserID = userID
		entries = append(entries, &entry)
	}

	return entries, nil
}

// MarkDigestEntriesSent records the entries as sent in a digest and updates
// the user's last digest timestamp. It never touches the entry status.
func (s *Storage) MarkDigestEntriesSent(userID int64, entryIDs []int64, sentAt time.Time) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf(`store: unable to start transaction: %v`, err)
	}
	defer tx.Rollback()

	for _, entryID := range entryIDs {
		if _, err := tx.Exec(
			`INSERT INTO digest_entries (user_id, entry_id, sent_at) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
			userID, entryID, sentAt,
		); err != nil {
			return fmt.Errorf(`store: unable to record digest entry for user #%d: %v`, userID, err)
		}
	}

	if _, err := tx.Exec(`UPDATE users SET last_digest_sent_at=$2 WHERE id=$1`, userID, sentAt); err != nil {
		return fmt.Errorf(`store: unable to update last digest date for user #%d: %v`, userID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf(`store: unable to commit transaction: %v`, err)
	}

	return nil
}
