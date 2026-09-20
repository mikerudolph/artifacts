package types

const (
	MinTTLSeconds = 60

	MaxTTLSeconds = 31_536_000

	DefaultTTLSeconds = 86_400
)

func ParseTTL(seconds int) (int, error) {
	if seconds == 0 {
		return DefaultTTLSeconds, nil
	}
	if seconds < MinTTLSeconds || seconds > MaxTTLSeconds {
		return 0, ErrInvalidTTL
	}
	return seconds, nil
}
