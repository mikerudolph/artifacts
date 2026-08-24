package types

const (
	// MinTTLSeconds is 1 minute.
	MinTTLSeconds = 60
	// MaxTTLSeconds is 1 year.
	MaxTTLSeconds = 31_536_000
	// DefaultTTLSeconds is 24 hours.
	DefaultTTLSeconds = 86_400
)

// ParseTTL validates a token TTL in seconds. Zero defaults to DefaultTTLSeconds.
func ParseTTL(seconds int) (int, error) {
	if seconds == 0 {
		return DefaultTTLSeconds, nil
	}
	if seconds < MinTTLSeconds || seconds > MaxTTLSeconds {
		return 0, ErrInvalidTTL
	}
	return seconds, nil
}
