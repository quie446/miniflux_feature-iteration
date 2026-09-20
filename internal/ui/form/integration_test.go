// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package form

import "testing"

func TestValidateDigestDisabled(t *testing.T) {
	integrationForm := &IntegrationForm{DigestEnabled: false, DigestIntervalHours: 0}
	if validationErr := integrationForm.ValidateDigest(); validationErr != nil {
		t.Fatalf(`A disabled digest should always be valid, got %v`, validationErr)
	}
}

func TestValidateDigestWithInvalidInterval(t *testing.T) {
	for _, interval := range []int{0, -1} {
		integrationForm := &IntegrationForm{
			DigestEnabled:       true,
			DigestIntervalHours: interval,
			WebhookEnabled:      true,
			WebhookURL:          "https://example.org/webhook",
		}
		if validationErr := integrationForm.ValidateDigest(); validationErr == nil {
			t.Errorf(`An enabled digest with interval %d should be rejected`, interval)
		}
	}
}

func TestValidateDigestRequiresWebhook(t *testing.T) {
	integrationForm := &IntegrationForm{DigestEnabled: true, DigestIntervalHours: 6}
	if validationErr := integrationForm.ValidateDigest(); validationErr == nil {
		t.Error(`An enabled digest without a configured webhook should be rejected`)
	}
}

func TestValidateDigestValid(t *testing.T) {
	integrationForm := &IntegrationForm{
		DigestEnabled:       true,
		DigestIntervalHours: 6,
		WebhookEnabled:      true,
		WebhookURL:          "https://example.org/webhook",
	}
	if validationErr := integrationForm.ValidateDigest(); validationErr != nil {
		t.Fatalf(`A valid digest configuration should be accepted, got %v`, validationErr)
	}
}
