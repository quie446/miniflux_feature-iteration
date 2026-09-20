// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package form // import "miniflux.app/v2/internal/ui/form"

import (
	"testing"
)

func TestValid(t *testing.T) {
	settings := &SettingsForm{
		Username:                "user",
		Password:                "hunter2",
		Confirmation:            "hunter2",
		Theme:                   "default",
		Language:                "en_US",
		Timezone:                "UTC",
		EntryDirection:          "asc",
		EntriesPerPage:          50,
		DisplayMode:             "standalone",
		GestureNav:              "tap",
		DefaultReadingSpeed:     35,
		CJKReadingSpeed:         25,
		DefaultHomePage:         "unread",
		MediaPlaybackRate:       1.25,
		AlwaysOpenExternalLinks: true,
	}

	err := settings.Validate()
	if err != nil {
		t.Error(err)
	}
}

func TestConfirmationEmpty(t *testing.T) {
	settings := &SettingsForm{
		Username:                "user",
		Password:                "hunter2",
		Confirmation:            "",
		Theme:                   "default",
		Language:                "en_US",
		Timezone:                "UTC",
		EntryDirection:          "asc",
		EntriesPerPage:          50,
		DisplayMode:             "standalone",
		GestureNav:              "tap",
		DefaultReadingSpeed:     35,
		CJKReadingSpeed:         25,
		DefaultHomePage:         "unread",
		MediaPlaybackRate:       1.25,
		AlwaysOpenExternalLinks: true,
	}

	err := settings.Validate()
	if err != nil {
		t.Error(err)
	}

	if settings.Password != "" {
		t.Error("Password should have been cleared")
	}
}

func TestConfirmationIncorrect(t *testing.T) {
	settings := &SettingsForm{
		Username:                "user",
		Password:                "hunter2",
		Confirmation:            "unter2",
		Theme:                   "default",
		Language:                "en_US",
		Timezone:                "UTC",
		EntryDirection:          "asc",
		EntriesPerPage:          50,
		DisplayMode:             "standalone",
		GestureNav:              "tap",
		DefaultReadingSpeed:     35,
		CJKReadingSpeed:         25,
		DefaultHomePage:         "unread",
		MediaPlaybackRate:       1.25,
		AlwaysOpenExternalLinks: true,
	}

	err := settings.Validate()
	if err == nil {
		t.Error("Validate should return an error")
	}
}

func TestDigestIntervalValidation(t *testing.T) {
	newSettingsForm := func(digestEnabled bool, digestIntervalHours int) *SettingsForm {
		return &SettingsForm{
			Username:            "user",
			Theme:               "default",
			Language:            "en_US",
			Timezone:            "UTC",
			EntryDirection:      "asc",
			EntriesPerPage:      50,
			DisplayMode:         "standalone",
			DefaultReadingSpeed: 35,
			CJKReadingSpeed:     25,
			DefaultHomePage:     "unread",
			MediaPlaybackRate:   1,
			DigestEnabled:       digestEnabled,
			DigestIntervalHours: digestIntervalHours,
		}
	}

	if err := newSettingsForm(true, 6).Validate(); err != nil {
		t.Errorf("a valid digest configuration should be accepted, got %v", err)
	}

	if err := newSettingsForm(false, 0).Validate(); err != nil {
		t.Errorf("a disabled digest with a zero interval should be accepted, got %v", err)
	}

	if err := newSettingsForm(true, 0).Validate(); err == nil {
		t.Error("a zero digest interval should be rejected when the digest is enabled")
	}

	if err := newSettingsForm(true, -2).Validate(); err == nil {
		t.Error("a negative digest interval should be rejected when the digest is enabled")
	}
}
