package service

import (
	"errors"
	"testing"
	"time"
)

func TestParseStatsRange(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name             string
		from, to, bucket string
		wantErr          bool
		wantBucket       string
		wantFrom, wantTo time.Time
	}{
		{
			name: "defaults to the last 30 days in day buckets",
			wantBucket: "day", wantTo: now, wantFrom: now.Add(-defaultStatsWindow),
		},
		{
			name: "accepts RFC3339",
			from: "2026-09-01T00:00:00Z", to: "2026-09-10T00:00:00Z", bucket: "day",
			wantBucket: "day",
			wantFrom:   time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
			wantTo:     time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "accepts a bare date as UTC midnight",
			from: "2026-09-01", to: "2026-09-10",
			wantBucket: "day",
			wantFrom:   time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
			wantTo:     time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC),
		},
		{name: "bucket is case insensitive", bucket: "HOUR", from: "2026-09-12", to: "2026-09-13", wantBucket: "hour"},
		{name: "unknown bucket", bucket: "fortnight", wantErr: true},
		{name: "unparseable from", from: "yesterday", wantErr: true},
		{name: "unparseable to", to: "soon", wantErr: true},
		{name: "from after to", from: "2026-09-10", to: "2026-09-01", wantErr: true},
		{name: "from equal to to", from: "2026-09-10", to: "2026-09-10", wantErr: true},
		{
			name: "hourly over a year busts the bucket cap",
			from: "2025-09-13", to: "2026-09-13", bucket: "hour", wantErr: true,
		},
		{
			name: "the same year in day buckets is fine",
			from: "2025-09-13", to: "2026-09-13", bucket: "day", wantBucket: "day",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseStatsRange(tt.from, tt.to, tt.bucket, now)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseStatsRange() = %+v, want error", got)
				}
				if !errors.Is(err, ErrInvalidStatsRange) {
					t.Fatalf("error = %v, want ErrInvalidStatsRange so the handler can map it to 400", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseStatsRange() error = %v", err)
			}
			if got.Bucket != tt.wantBucket {
				t.Errorf("Bucket = %q, want %q", got.Bucket, tt.wantBucket)
			}
			if got.Unit == "" || got.Step == "" {
				t.Errorf("Unit/Step = %q/%q, want both resolved from the bucket table", got.Unit, got.Step)
			}
			if !tt.wantFrom.IsZero() && !got.From.Equal(tt.wantFrom) {
				t.Errorf("From = %v, want %v", got.From, tt.wantFrom)
			}
			if !tt.wantTo.IsZero() && !got.To.Equal(tt.wantTo) {
				t.Errorf("To = %v, want %v", got.To, tt.wantTo)
			}
		})
	}
}

// The cap is what keeps an indefinitely-retained table from being scanned into
// an unrenderable chart, so pin its boundary rather than just its existence.
func TestParseStatsRangeBucketCapBoundary(t *testing.T) {
	now := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	at := func(hours int) string {
		return now.Add(-time.Duration(hours) * time.Hour).Format(time.RFC3339)
	}

	if _, err := ParseStatsRange(at(maxStatsBuckets), now.Format(time.RFC3339), "hour", now); err != nil {
		t.Errorf("exactly maxStatsBuckets buckets should be allowed, got %v", err)
	}
	if _, err := ParseStatsRange(at(maxStatsBuckets+1), now.Format(time.RFC3339), "hour", now); err == nil {
		t.Error("one bucket over the cap should be rejected")
	}
}
