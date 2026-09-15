package main

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthzHandler(t *testing.T) {
	lastGatewayConnectUnixNano.Store(time.Now().UnixNano())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/healthz", nil)
	healthzHandler(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200 for a recent gateway connection, got %d", rec.Code)
	}

	stale := time.Now().Add(-gatewayHealthyWindow - time.Minute)
	lastGatewayConnectUnixNano.Store(stale.UnixNano())

	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/healthz", nil)
	healthzHandler(rec, req)

	if rec.Code != 503 {
		t.Fatalf("expected 503 for a stale gateway connection, got %d", rec.Code)
	}
}
