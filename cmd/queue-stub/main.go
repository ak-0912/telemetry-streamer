// Binary queue-stub is a lightweight in-memory gRPC message queue used for
// local development and integration testing of the telemetry-streamer.
package main

import (
	"context"
	"log/slog"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	mqv1 "telemetry-streamer/api/mq/v1"
)

type stubQueue struct {
	mu                sync.Mutex
	depth             int
	capacity          int
	rejectUtilization float64
	failEnqueuePct    int
}

type mqImpl struct {
	mqv1.UnimplementedMessageQueueServiceServer
	q *stubQueue
}

func (s *mqImpl) Publish(_ context.Context, in *mqv1.PublishRequest) (*mqv1.PublishResponse, error) {
	_ = in.GetTopic()
	_ = in.GetKey()
	_ = in.GetPayload()

	accepted, depth, util := s.q.enqueue()
	if !accepted {
		return nil, status.Errorf(codes.ResourceExhausted, "queue overloaded util=%.3f", util)
	}
	return &mqv1.PublishResponse{Partition: 0, Offset: int64(depth)}, nil
}

func main() {
	addr := getString("STUB_ADDR", ":8081")
	capacity := getInt("STUB_CAPACITY", 1024)
	if capacity <= 0 {
		capacity = 1024
	}
	rejectUtil := getFloat("STUB_REJECT_UTILIZATION", 0.95)
	if rejectUtil <= 0 || rejectUtil > 1 {
		rejectUtil = 0.95
	}
	consumeEvery := getDurationMS("STUB_CONSUME_INTERVAL_MS", 75)
	failPct := getInt("STUB_FAIL_ENQUEUE_PCT", 0)
	if failPct < 0 {
		failPct = 0
	}
	if failPct > 100 {
		failPct = 100
	}

	q := &stubQueue{
		capacity:          capacity,
		rejectUtilization: rejectUtil,
		failEnqueuePct:    failPct,
	}

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("failed to listen", "addr", addr, "err", err)
		os.Exit(1)
	}

	grpcSrv := grpc.NewServer()
	mqv1.RegisterMessageQueueServiceServer(grpcSrv, &mqImpl{q: q})

	stopDrain := make(chan struct{})
	go q.runDrainLoop(stopDrain, consumeEvery)

	go func() {
		slog.Info("queue-stub listening", "addr", lis.Addr().String(), "service", "mq.v1.MessageQueueService")
		if err := grpcSrv.Serve(lis); err != nil {
			slog.Error("grpc server stopped", "err", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	close(stopDrain)
	grpcSrv.GracefulStop()
}

func (q *stubQueue) enqueue() (accepted bool, depth int, util float64) {
	q.mu.Lock()
	defer q.mu.Unlock()

	util = q.utilizationLocked()
	if q.failEnqueuePct > 0 && rand.Intn(100) < q.failEnqueuePct {
		return false, q.depth, util
	}
	if util >= q.rejectUtilization || q.depth >= q.capacity {
		return false, q.depth, util
	}
	q.depth++
	util = q.utilizationLocked()
	return true, q.depth, util
}

func (q *stubQueue) runDrainLoop(stop <-chan struct{}, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			q.mu.Lock()
			if q.depth > 0 {
				q.depth--
			}
			q.mu.Unlock()
		}
	}
}

func (q *stubQueue) utilizationLocked() float64 {
	if q.capacity <= 0 {
		return 0
	}
	return float64(q.depth) / float64(q.capacity)
}

func getString(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}

func getFloat(key string, fallback float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return f
}

func getDurationMS(key string, fallbackMS int) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return time.Duration(fallbackMS) * time.Millisecond
	}
	ms, err := strconv.Atoi(value)
	if err != nil {
		return time.Duration(fallbackMS) * time.Millisecond
	}
	return time.Duration(ms) * time.Millisecond
}
