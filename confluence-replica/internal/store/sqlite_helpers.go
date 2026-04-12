package store

import "time"

func currentTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func storeTime(t time.Time) string {
	if t.IsZero() {
		return currentTimestamp()
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func mustParseTime(raw string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02 15:04:05Z07:00"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
