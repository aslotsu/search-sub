package main

import (
	"errors"
	"testing"
	"time"
)

func TestIsRetryableTypesenseError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "serverless not ready", err: errors.New(`status: 503 response: { "message": "Not Ready or Lagging"}`), want: true},
		{name: "rate limited", err: errors.New("status: 429 response: rate limited"), want: true},
		{name: "gateway timeout", err: errors.New("status: 504 response: timeout"), want: true},
		{name: "network timeout", err: errors.New("dial tcp: i/o timeout"), want: true},
		{name: "validation error", err: errors.New("status: 400 response: missing required field"), want: false},
		{name: "not found", err: errors.New("status: 404 response: document not found"), want: false},
		{name: "nil", err: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryableTypesenseError(tt.err)
			if got != tt.want {
				t.Fatalf("isRetryableTypesenseError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestRetryDelayCapsAtMax(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 1, want: time.Second},
		{attempt: 2, want: 2 * time.Second},
		{attempt: 5, want: 16 * time.Second},
		{attempt: 10, want: typesenseMaxRetryDelay},
	}

	for _, tt := range tests {
		got := retryDelay(tt.attempt)
		if got != tt.want {
			t.Fatalf("retryDelay(%d) = %s, want %s", tt.attempt, got, tt.want)
		}
	}
}
