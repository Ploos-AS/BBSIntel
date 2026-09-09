package sourcehealth

import "time"

const (
	FreshMaxAge    = 36 * time.Hour
	DegradedMaxAge = 72 * time.Hour
)

type Classification struct {
	State      string
	AgeSeconds int64
}

func Classify(now time.Time, lastSuccess time.Time, consecutiveFailures int) Classification {
	if lastSuccess.IsZero() {
		if consecutiveFailures > 0 {
			return Classification{State: "failed"}
		}
		return Classification{State: "unknown"}
	}

	age := now.Sub(lastSuccess)
	if age < 0 {
		age = 0
	}
	out := Classification{AgeSeconds: int64(age / time.Second)}

	switch {
	case consecutiveFailures >= 3:
		out.State = "failed"
	case age > DegradedMaxAge:
		out.State = "stale"
	case consecutiveFailures > 0 || age > FreshMaxAge:
		out.State = "degraded"
	default:
		out.State = "fresh"
	}
	return out
}
