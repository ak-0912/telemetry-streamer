package telemetry

import (
	"testing"
	"time"
)

func TestFromCSVRecord(t *testing.T) {
	record := CSVRecord{
		MetricName: "DCGM_FI_DEV_GPU_UTIL",
		GPUID:      "0",
		Device:     "nvidia0",
		UUID:       "gpu-uuid",
		ModelName:  "H100",
		Hostname:   "host-a",
		Value:      42.5,
		LabelsRaw:  `gpu="0"`,
	}
	now := time.Now().UTC()

	reading := FromCSVRecord(record, now)

	if reading.MetricName != record.MetricName || reading.GPUID != record.GPUID || reading.Value != record.Value {
		t.Fatalf("reading fields were not copied from csv record")
	}
	if !reading.Timestamp.Equal(now) {
		t.Fatalf("timestamp mismatch got=%v want=%v", reading.Timestamp, now)
	}
}

func TestValidateReading(t *testing.T) {
	valid := Reading{MetricName: "metric", GPUID: "0"}
	if err := ValidateReading(valid); err != nil {
		t.Fatalf("expected valid reading, got error: %v", err)
	}

	cases := []Reading{
		{MetricName: "", GPUID: "0"},
		{MetricName: "metric", GPUID: ""},
	}
	for _, tc := range cases {
		if err := ValidateReading(tc); err == nil {
			t.Fatalf("expected invalid reading error for %+v", tc)
		}
	}
}
