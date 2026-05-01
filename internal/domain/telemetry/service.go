package telemetry

import "errors"

var ErrInvalidReading = errors.New("invalid telemetry reading")

// ValidateReading keeps core business rules in the domain layer.
func ValidateReading(r Reading) error {
	if r.GPUId == "" {
		return ErrInvalidReading
	}
	if r.MetricName == "" {
		return ErrInvalidReading
	}
	return nil
}
