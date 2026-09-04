package models

import "time"

// TimeLayout is the fixed-width, nanosecond-precision RFC 3339 layout every
// timestamp field on a model in this package is written and read with.
//
// Plain time.RFC3339 (second precision) loses ordering between two writes in
// the same second, and time.RFC3339Nano is unsafe too: Go trims trailing
// fractional zeros, so two timestamps with different digit counts stop
// comparing correctly as plain strings. A fixed nine-digit fractional part
// keeps every stored timestamp both parseable and lexicographically
// sortable in the same order as chronological — see
// mwanachama-backend-actor/models/time.go, which this mirrors.
const TimeLayout = "2006-01-02T15:04:05.000000000Z07:00"

// NowRFC3339 returns the current UTC time formatted per [TimeLayout].
func NowRFC3339() string {
	return time.Now().UTC().Format(TimeLayout)
}
