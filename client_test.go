package raven

import "testing"

// TestClient_SetDSN verifies the DSN is successfully parsed and applied to Client.
func TestClient_SetDSN(t *testing.T) {
	client := &Client{}
	client.SetDSN("https://u:p@example.com/sentry/1")

	if client.url != "https://example.com/sentry/api/1/store/" {
		t.Error("incorrect url:", client.url)
	}
	if client.projectId != "1" {
		t.Error("incorrect projectId:", client.projectId)
	}
	if client.authHeader != "Sentry sentry_version=4, sentry_key=u, sentry_secret=p" {
		t.Error("incorrect authHeader:", client.authHeader)
	}
}
