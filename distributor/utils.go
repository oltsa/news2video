package main

import (
	"fmt"
	"math"
	"time"

	"github.com/rs/zerolog"
)

// RetryableFunc is a function that can be retried.
type RetryableFunc func() error

// RetryWithBackoff executes a function with an exponential backoff retry strategy.
func RetryWithBackoff(log zerolog.Logger, description string, fn RetryableFunc, maxRetries int, initialDelay time.Duration) error {
	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		err = fn()
		if err == nil {
			return nil // Success
		}

		delay := initialDelay * time.Duration(math.Pow(2, float64(attempt)))
		log.Warn().
			Err(err).
			Int("attempt", attempt+1).
			Int("max_retries", maxRetries).
			Dur("delay", delay).
			Msgf("Retrying operation: %s", description)
		time.Sleep(delay)
	}
	return fmt.Errorf("operation failed after %d attempts: %w", maxRetries, err)
}
