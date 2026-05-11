// Package csv implements the TelemetryReader port by parsing a DCGM metrics
// CSV file into domain Readings and replaying them in an infinite loop.
package csv

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"telemetry-streamer/internal/domain/telemetry"
)

var errEmptyCSV = errors.New("csv input has no data rows")

// Reader holds pre-parsed CSV records in memory and replays them indefinitely
// on each call to Read, stamping each with the current processing time.
type Reader struct {
	records    []telemetry.CSVRecord
	shardTotal int
	shardIndex int
}

// NewReader creates a Reader that processes the entire CSV (single shard).
func NewReader(filePath string) (*Reader, error) {
	return NewReaderWithShard(filePath, 1, 0)
}

// NewReaderWithShard creates a Reader that only processes rows where rowIndex%shardTotal == shardIndex.
// This allows horizontal scaling by splitting the dataset across multiple streamer replicas.
func NewReaderWithShard(filePath string, shardTotal, shardIndex int) (*Reader, error) {
	if shardTotal <= 0 {
		return nil, fmt.Errorf("invalid shard_total=%d: must be > 0", shardTotal)
	}
	if shardIndex < 0 || shardIndex >= shardTotal {
		return nil, fmt.Errorf("invalid shard_index=%d: must be in [0, %d)", shardIndex, shardTotal)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open csv: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	parser := csv.NewReader(file)
	// DCGM CSV exports may contain unbalanced quotes and rows with varying
	// column counts (e.g. labels_raw embeds commas); these settings prevent
	// hard parse failures on real-world DCGM output.
	parser.LazyQuotes = true
	parser.FieldsPerRecord = -1

	header, err := parser.Read()
	if err != nil {
		return nil, fmt.Errorf("read csv header: %w", err)
	}

	indexByName := map[string]int{}
	for i, name := range header {
		indexByName[strings.ToLower(strings.TrimSpace(name))] = i
	}

	requiredColumns := []string{"metric_name", "gpu_id", "device", "uuid", "modelname", "hostname", "value", "labels_raw"}
	for _, col := range requiredColumns {
		if _, ok := indexByName[col]; !ok {
			return nil, fmt.Errorf("missing required column: %s", col)
		}
	}

	records := make([]telemetry.CSVRecord, 0, 1024)
	rowNum := 1 // header was row 1
	for {
		row, readErr := parser.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		rowNum++
		if readErr != nil {
			return nil, fmt.Errorf("read csv row %d: %w", rowNum, readErr)
		}

		record, parseErr := parseRow(row, indexByName)
		if parseErr != nil {
			slog.Warn("csv skip row", "row", rowNum, "reason", parseErr)
			continue
		}
		records = append(records, record)
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("%w: no valid data rows after parsing (check value column and quoting)", errEmptyCSV)
	}

	shardRecords := records
	if shardTotal > 1 {
		filtered := make([]telemetry.CSVRecord, 0, len(records))
		for i, rec := range records {
			if i%shardTotal == shardIndex {
				filtered = append(filtered, rec)
			}
		}
		shardRecords = filtered
	}

	slog.Info("csv reader initialized",
		"path", filePath,
		"records_total", len(records),
		"records_shard", len(shardRecords),
		"shard_index", shardIndex,
		"shard_total", shardTotal,
	)
	return &Reader{
		records:    shardRecords,
		shardTotal: shardTotal,
		shardIndex: shardIndex,
	}, nil
}

// Read starts an infinite replay loop, emitting one Reading per record on the returned channel.
// The loop restarts from the beginning when all records have been sent.
// Cancelling ctx stops the loop and closes both channels.
func (r *Reader) Read(ctx context.Context) (<-chan telemetry.Reading, <-chan error) {
	out := make(chan telemetry.Reading)
	errs := make(chan error)

	go func() {
		defer close(out)
		defer close(errs)
		if len(r.records) == 0 {
			slog.Warn("csv reader shard has no assigned records", "shard_index", r.shardIndex, "shard_total", r.shardTotal)
			<-ctx.Done()
			return
		}
		i := 0
		totalSent := 0
		for {
			record := r.records[i]
			reading := telemetry.FromCSVRecord(record, time.Now().UTC())

			select {
			case <-ctx.Done():
				return
			case out <- reading:
			}
			totalSent++
			if totalSent == 1 || totalSent%1000 == 0 {
				slog.Info("csv reader emitted", "records", totalSent)
			}

			i++
			if i >= len(r.records) {
				slog.Debug("csv reader loop restart", "consumed", totalSent, "dataset_size", len(r.records))
				i = 0
			}
		}
	}()

	return out, errs
}

// normalizeFloatString trims spaces and a single pair of ASCII double quotes often
// added by spreadsheet/CSV exporters around numeric cells.
func normalizeFloatString(s string) string {
	s = strings.TrimSpace(s)
	for len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	return strings.TrimSpace(s)
}

func parseRow(row []string, indexes map[string]int) (telemetry.CSVRecord, error) {
	get := func(key string) string {
		idx := indexes[key]
		if idx >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[idx])
	}

	parseFloat := func(name, value string) (float64, error) {
		normalized := normalizeFloatString(value)
		if normalized == "" {
			return 0, fmt.Errorf("parse %s: empty value (row may be missing columns so value shifted into labels)", name)
		}
		number, err := strconv.ParseFloat(normalized, 64)
		if err != nil {
			return 0, fmt.Errorf("csv %s: invalid numeric %q: %w", name, normalized, err)
		}
		return number, nil
	}

	val, err := parseFloat("value", get("value"))
	if err != nil {
		return telemetry.CSVRecord{}, err
	}

	return telemetry.CSVRecord{
		MetricName: get("metric_name"),
		GPUID:      get("gpu_id"),
		Device:     get("device"),
		UUID:       get("uuid"),
		ModelName:  get("modelname"),
		Hostname:   get("hostname"),
		Value:      val,
		LabelsRaw:  get("labels_raw"),
	}, nil
}
