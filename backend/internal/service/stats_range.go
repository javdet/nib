package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/javdet/nib/internal/domain"
)

// ErrInvalidStatsRange rejects a statistics window the service will not answer.
// It needs a case in handler.handleServiceError or it degrades to a 500.
var ErrInvalidStatsRange = errors.New("invalid statistics range")

// bucketSpec pairs the date_trunc unit with the generate_series step.
//
// Both come from this table and never from the query string, so neither value
// is ever concatenated into SQL from user input.
type bucketSpec struct{ unit, step string }

var bucketSpecs = map[string]bucketSpec{
	"hour":  {"hour", "1 hour"},
	"day":   {"day", "1 day"},
	"week":  {"week", "1 week"},
	"month": {"month", "1 month"},
}

const (
	defaultStatsBucket = "day"
	defaultStatsWindow = 30 * 24 * time.Hour

	// maxStatsBuckets bounds both the scan and the chart. Statistics are kept
	// indefinitely, so hourly buckets over a few years would otherwise be a
	// five-figure row count that no chart can render and no user asked for.
	maxStatsBuckets = 400
)

// bucketDuration is the nominal length of one bucket, used only to reject a
// request before it runs. Months vary and DST shifts hours, so this is an
// estimate -- it guards the cap, it does not define the buckets.
var bucketDuration = map[string]time.Duration{
	"hour":  time.Hour,
	"day":   24 * time.Hour,
	"week":  7 * 24 * time.Hour,
	"month": 30 * 24 * time.Hour,
}

// ParseStatsRange validates a requested window and resolves it to the units the
// repository needs. now is injected so the defaults are testable.
//
// from/to accept RFC3339 or a bare YYYY-MM-DD (treated as UTC midnight), which
// keeps hand-written URLs and e2e mocks readable.
func ParseStatsRange(fromRaw, toRaw, bucketRaw string, now time.Time) (domain.StatsRange, error) {
	bucket := strings.ToLower(strings.TrimSpace(bucketRaw))
	if bucket == "" {
		bucket = defaultStatsBucket
	}
	spec, ok := bucketSpecs[bucket]
	if !ok {
		return domain.StatsRange{}, fmt.Errorf("%w: bucket must be one of hour, day, week, month", ErrInvalidStatsRange)
	}

	to, err := parseStatsTime(toRaw, now.UTC())
	if err != nil {
		return domain.StatsRange{}, fmt.Errorf("%w: to: %s", ErrInvalidStatsRange, err)
	}
	from, err := parseStatsTime(fromRaw, to.Add(-defaultStatsWindow))
	if err != nil {
		return domain.StatsRange{}, fmt.Errorf("%w: from: %s", ErrInvalidStatsRange, err)
	}

	if !from.Before(to) {
		return domain.StatsRange{}, fmt.Errorf("%w: from must be before to", ErrInvalidStatsRange)
	}
	if n := to.Sub(from) / bucketDuration[bucket]; n > maxStatsBuckets {
		return domain.StatsRange{}, fmt.Errorf(
			"%w: that range is about %d %s buckets, more than the %d allowed -- widen the bucket or shorten the range",
			ErrInvalidStatsRange, n, bucket, maxStatsBuckets)
	}

	return domain.StatsRange{
		From: from, To: to, Bucket: bucket, Unit: spec.unit, Step: spec.step,
	}, nil
}

func parseStatsTime(raw string, fallback time.Time) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("must be RFC3339 or YYYY-MM-DD, got %q", raw)
}
