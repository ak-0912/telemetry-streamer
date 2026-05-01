package csv

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"telemetry-streamer/internal/domain/telemetry"
)

var errEmptyCSV = errors.New("csv input has no data rows")

// Reader loads telemetry records and loops forever.
type Reader struct {
	records []telemetry.CSVRecord
}

func NewReader(filePath string) (*Reader, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open csv: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	parser := csv.NewReader(file)

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
	for {
		row, readErr := parser.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("read csv row: %w", readErr)
		}

		record, parseErr := parseRow(row, indexByName)
		if parseErr != nil {
			return nil, parseErr
		}
		records = append(records, record)
	}

	if len(records) == 0 {
		return nil, errEmptyCSV
	}

	log.Printf("csv reader initialized: path=%s records=%d", filePath, len(records))
	return &Reader{records: records}, nil
}

func (r *Reader) Read(ctx context.Context) (<-chan telemetry.Reading, <-chan error) {
	out := make(chan telemetry.Reading)
	errs := make(chan error)

	go func() {
		defer close(out)
		defer close(errs)
		i := 0
		totalSent := 0
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			record := r.records[i]
			reading := telemetry.FromCSVRecord(record, time.Now().UTC())

			select {
			case <-ctx.Done():
				return
			case out <- reading:
			}
			totalSent++
			if totalSent == 1 || totalSent%1000 == 0 {
				log.Printf("csv reader emitted records=%d", totalSent)
			}

			i++
			if i >= len(r.records) {
				log.Printf("csv reader loop restart: consumed=%d dataset_size=%d", totalSent, len(r.records))
				i = 0
			}
		}
	}()

	return out, errs
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
		number, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0, fmt.Errorf("parse %s=%q: %w", name, value, err)
		}
		return number, nil
	}

	val, err := parseFloat("value", get("value"))
	if err != nil {
		return telemetry.CSVRecord{}, err
	}

	return telemetry.CSVRecord{
		MetricName: get("metric_name"),
		GPUId:      get("gpu_id"),
		Device:     get("device"),
		UUID:       get("uuid"),
		ModelName:  get("modelname"),
		HostName:   get("hostname"),
		Value:      val,
		LabelsRaw:  get("labels_raw"),
	}, nil
}
