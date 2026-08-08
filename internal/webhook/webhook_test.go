// Copyright 2026 Steven Crothers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package webhook_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scrothers/pmmcp/internal/webhook"
)

func TestRegistryCRUD(t *testing.T) {
	t.Parallel()
	r := webhook.NewRegistry(webhook.WithAllowlist("*.example.com"))
	h := webhook.Hook{
		ID:     "hook-1",
		URL:    "https://example.com/hooks/pmmcp",
		Events: []string{"process.crashed"},
	}
	if err := r.Create(h); err != nil {
		t.Fatalf("Create: %v", err)
	}
	list := r.List()
	if len(list) != 1 || list[0].ID != "hook-1" {
		t.Fatalf("List = %+v", list)
	}
	got, err := r.Get("hook-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.URL != h.URL {
		t.Fatalf("URL = %q", got.URL)
	}
	if err := r.Delete("hook-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := r.Delete("hook-1"); !errors.Is(err, webhook.ErrNotFound) {
		t.Fatalf("Delete missing = %v", err)
	}
}

func TestCreateRejectsMetadata(t *testing.T) {
	t.Parallel()
	// Even when the metadata IP is explicitly allowlisted, the SSRF guard blocks it.
	r := webhook.NewRegistry(webhook.WithAllowlist("169.254.169.254"))
	err := r.Create(webhook.Hook{
		ID:  "bad",
		URL: "http://169.254.169.254/latest/meta-data/",
	})
	if !errors.Is(err, webhook.ErrSSRF) {
		t.Fatalf("Create metadata err = %v, want ErrSSRF", err)
	}
}

func TestCreateEmptyAllowlistDenies(t *testing.T) {
	t.Parallel()
	r := webhook.NewRegistry() // no allowlist ⇒ webhooks disabled
	err := r.Create(webhook.Hook{ID: "x", URL: "https://example.com/hook"})
	if !errors.Is(err, webhook.ErrSSRF) {
		t.Fatalf("empty-allowlist Create = %v, want ErrSSRF", err)
	}
}

func TestValidateURLSSRF(t *testing.T) {
	t.Parallel()
	blocked := []string{
		"http://169.254.169.254/latest/meta-data/",
		"https://169.254.169.254/",
		"http://127.0.0.1:8080/hook",
		"http://localhost/hook",
		"http://[::1]/hook",
		"file:///etc/passwd",
		"ftp://example.com/x",
		"http://0.0.0.0/",
	}
	for _, raw := range blocked {
		err := webhook.ValidateURL(raw)
		if !errors.Is(err, webhook.ErrSSRF) && !errors.Is(err, webhook.ErrInvalid) {
			t.Fatalf("ValidateURL(%q) = %v, want ErrSSRF/ErrInvalid", raw, err)
		}
	}
	if err := webhook.ValidateURL("https://example.com/hook"); errors.Is(err, webhook.ErrSSRF) {
		t.Fatalf("example.com blocked: %v", err)
	}
}

func TestDeliverRejectMetadata(t *testing.T) {
	t.Parallel()
	err := webhook.Deliver(context.Background(), webhook.Hook{
		ID:  "meta",
		URL: "http://169.254.169.254/latest/meta-data/",
	}, map[string]string{"x": "1"})
	if !errors.Is(err, webhook.ErrSSRF) {
		t.Fatalf("err = %v, want ErrSSRF", err)
	}
}

func TestDeliverRejectLoopbackDefault(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	err := webhook.Deliver(context.Background(), webhook.Hook{
		ID:  "local",
		URL: srv.URL,
	}, map[string]string{"x": "1"})
	if !errors.Is(err, webhook.ErrSSRF) {
		t.Fatalf("err = %v, want ErrSSRF", err)
	}
}
