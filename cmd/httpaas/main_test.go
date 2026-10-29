package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSalud(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		wantCode int
		wantBody string
	}{
		{name: "get returns ok", method: http.MethodGet, wantCode: http.StatusOK, wantBody: "ok"},
		{name: "post is not allowed", method: http.MethodPost, wantCode: http.StatusMethodNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newMux().ServeHTTP(rec, httptest.NewRequest(tt.method, "/salud", nil))
			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if tt.wantBody != "" && rec.Body.String() != tt.wantBody {
				t.Fatalf("body = %q, want %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}
