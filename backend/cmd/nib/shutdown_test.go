package main

import (
	"testing"
	"time"
)

func TestShutdownReserve(t *testing.T) {
	tests := []struct {
		budget, want time.Duration
	}{
		{0, 0},
		{10 * time.Second, 5 * time.Second},
		{30 * time.Second, 15 * time.Second},
		{40 * time.Second, 20 * time.Second},
		{15 * time.Minute, 20 * time.Second},
	}
	for _, tc := range tests {
		if got := shutdownReserve(tc.budget); got != tc.want {
			t.Errorf("shutdownReserve(%s) = %s, want %s", tc.budget, got, tc.want)
		}
	}
}
