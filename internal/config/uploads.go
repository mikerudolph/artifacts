package config

import (
	"fmt"
	"strconv"
	"time"
)

type Uploads struct {
	MaxBytes   int64
	Concurrent int
	Timeout    time.Duration
}

func (u Uploads) Defaults() Uploads {
	if u.MaxBytes == 0 {
		u.MaxBytes = 512 << 20
	}
	if u.Concurrent == 0 {
		u.Concurrent = 2
	}
	if u.Timeout == 0 {
		u.Timeout = 15 * time.Minute
	}
	return u
}

func loadUploads() (Uploads, error) {
	size, err := strconv.ParseInt(env("ARTIFACTS_COMMIT_MAX_BYTES", "536870912"), 10, 64)
	if err != nil {
		return Uploads{}, fmt.Errorf("ARTIFACTS_COMMIT_MAX_BYTES must be an integer")
	}
	count, err := strconv.Atoi(env("ARTIFACTS_COMMIT_CONCURRENCY", "2"))
	if err != nil {
		return Uploads{}, fmt.Errorf("ARTIFACTS_COMMIT_CONCURRENCY must be an integer")
	}
	u := Uploads{MaxBytes: size, Concurrent: count, Timeout: durationEnv("ARTIFACTS_COMMIT_TIMEOUT", 15*time.Minute)}
	return u, u.validate()
}

func (u Uploads) validate() error {
	if u.MaxBytes <= 0 || u.MaxBytes > 512<<20 {
		return fmt.Errorf("ARTIFACTS_COMMIT_MAX_BYTES must be between 1 and 536870912")
	}
	if u.Concurrent <= 0 || u.Concurrent > 64 {
		return fmt.Errorf("ARTIFACTS_COMMIT_CONCURRENCY must be between 1 and 64")
	}
	if u.Timeout <= 0 || u.Timeout > time.Hour {
		return fmt.Errorf("ARTIFACTS_COMMIT_TIMEOUT must be positive and at most 1h")
	}
	return nil
}
