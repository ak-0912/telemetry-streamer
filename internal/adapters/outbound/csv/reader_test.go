package csv

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewReaderAndReadLoops(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "metrics.csv")
	content := "metric_name,gpu_id,device,uuid,modelName,Hostname,value,labels_raw\n" +
		"DCGM_FI_DEV_GPU_UTIL,0,nvidia0,uuid-0,H100,host-a,10,gpu=0\n" +
		"DCGM_FI_DEV_GPU_UTIL,1,nvidia1,uuid-1,H100,host-a,20,gpu=1\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	reader, err := NewReader(path)
	if err != nil {
		t.Fatalf("new reader: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	readings, errs := reader.Read(ctx)

	got := make([]string, 0, 3)
	for range 3 {
		select {
		case err := <-errs:
			if err != nil {
				t.Fatalf("unexpected read error: %v", err)
			}
		case r := <-readings:
			got = append(got, r.GPUID)
			if r.Timestamp.IsZero() {
				t.Fatalf("expected processing timestamp to be set")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for readings")
		}
	}

	// Third reading should loop back to first row.
	want := []string{"0", "1", "0"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected loop order at index %d: got=%s want=%s", i, got[i], want[i])
		}
	}
}

func TestNewReaderSkipsInvalidValueRows(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "metrics.csv")
	content := "metric_name,gpu_id,device,uuid,modelName,Hostname,value,labels_raw\n" +
		"DCGM_FI_DEV_GPU_UTIL,0,nvidia0,uuid-0,H100,host-a,10,gpu=0\n" +
		"DCGM_FI_DEV_GPU_UTIL,0,nvidia0,uuid-0,H100,host-a,instance=\"bad:9400\",gpu=0\n" +
		"DCGM_FI_DEV_GPU_UTIL,1,nvidia1,uuid-1,H100,host-a,20,gpu=1\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	reader, err := NewReader(path)
	if err != nil {
		t.Fatalf("new reader: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	readings, errs := reader.Read(ctx)

	want := []string{"0", "1", "0"}
	for i := range want {
		select {
		case err := <-errs:
			if err != nil {
				t.Fatalf("unexpected read error: %v", err)
			}
		case r := <-readings:
			if r.GPUID != want[i] {
				t.Fatalf("index %d: gpu got=%s want=%s", i, r.GPUID, want[i])
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for readings")
		}
	}
}

func TestNewReaderQuotedNumericValue(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "metrics.csv")
	content := "metric_name,gpu_id,device,uuid,modelName,Hostname,value,labels_raw\n" +
		"DCGM_FI_DEV_GPU_UTIL,0,nvidia0,uuid-0,H100,host-a,\"99.5\",gpu=0\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	reader, err := NewReader(path)
	if err != nil {
		t.Fatalf("new reader: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	readings, _ := reader.Read(ctx)
	r := <-readings
	if r.Value != 99.5 {
		t.Fatalf("value got=%v want=99.5", r.Value)
	}
	cancel()
}

func TestNewReaderMissingColumn(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.csv")
	content := "metric_name,gpu_id,device,uuid,modelName,Hostname,labels_raw\n" // value missing
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	if _, err := NewReader(path); err == nil {
		t.Fatal("expected error for missing required column")
	}
}

func TestNewReaderWithShardFiltersRecords(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "metrics.csv")
	content := "metric_name,gpu_id,device,uuid,modelName,Hostname,value,labels_raw\n" +
		"DCGM_FI_DEV_GPU_UTIL,0,nvidia0,uuid-0,H100,host-a,10,gpu=0\n" +
		"DCGM_FI_DEV_GPU_UTIL,1,nvidia1,uuid-1,H100,host-a,20,gpu=1\n" +
		"DCGM_FI_DEV_GPU_UTIL,2,nvidia2,uuid-2,H100,host-a,30,gpu=2\n" +
		"DCGM_FI_DEV_GPU_UTIL,3,nvidia3,uuid-3,H100,host-a,40,gpu=3\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	reader, err := NewReaderWithShard(path, 2, 1)
	if err != nil {
		t.Fatalf("new reader with shard: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	readings, errs := reader.Read(ctx)

	want := []string{"1", "3", "1"}
	for i := range want {
		select {
		case err := <-errs:
			if err != nil {
				t.Fatalf("unexpected read error: %v", err)
			}
		case r := <-readings:
			if r.GPUID != want[i] {
				t.Fatalf("index %d: gpu got=%s want=%s", i, r.GPUID, want[i])
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for readings")
		}
	}
}

func TestNewReaderWithShardRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	if _, err := NewReaderWithShard("does-not-matter.csv", 0, 0); err == nil {
		t.Fatal("expected error for shard_total <= 0")
	}
	if _, err := NewReaderWithShard("does-not-matter.csv", 2, 2); err == nil {
		t.Fatal("expected error for shard_index out of range")
	}
}
