package telemetry

import (
	"errors"
	"fmt"
)

// ErrInvalidReading is returned when a Reading fails domain validation.
var ErrInvalidReading = errors.New("invalid telemetry reading")

// ValidateReading enforces core business rules: every reading must identify the GPU and the metric.
func ValidateReading(r Reading) error {
	if r.GPUID == "" {
		return fmt.Errorf("%w: GPUID is required", ErrInvalidReading)
	}
	if r.MetricName == "" {
		return fmt.Errorf("%w: MetricName is required", ErrInvalidReading)
	}
	return nil
}
