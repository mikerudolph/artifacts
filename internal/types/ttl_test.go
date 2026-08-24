package types

import "testing"

func TestParseTTL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      int
		want    int
		wantErr bool
	}{
		{0, DefaultTTLSeconds, false},
		{60, 60, false},
		{3600, 3600, false},
		{MaxTTLSeconds, MaxTTLSeconds, false},
		{59, 0, true},
		{MaxTTLSeconds + 1, 0, true},
		{-1, 0, true},
	}
	for _, tc := range cases {
		got, err := ParseTTL(tc.in)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Fatalf("ParseTTL(%d) = %d, %v", tc.in, got, err)
		}
	}
}
