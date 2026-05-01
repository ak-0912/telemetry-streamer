package main

import (
	"context"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	enqueueProcedure = "/telemetry.v1.QueueService/Enqueue"
	healthProcedure  = "/telemetry.v1.QueueService/Health"
)

type stubQueue struct {
	mu                sync.Mutex
	depth             int
	capacity          int
	rejectUtilization float64
	failEnqueuePct    int
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

	mux := http.NewServeMux()
	mux.Handle(enqueueProcedure, connect.NewUnaryHandler(
		enqueueProcedure,
		func(ctx context.Context, req *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
			_ = ctx
			_ = req
			accepted, depth, util := q.enqueue()
			resp, err := structpb.NewStruct(map[string]any{
				"accepted":    accepted,
				"depth":       float64(depth),
				"capacity":    float64(q.capacity),
				"utilization": util,
			})
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}
			return connect.NewResponse(resp), nil
		},
	))
	mux.Handle(healthProcedure, connect.NewUnaryHandler(
		healthProcedure,
		func(ctx context.Context, req *connect.Request[emptypb.Empty]) (*connect.Response[structpb.Struct], error) {
			_ = ctx
			_ = req
			depth, util := q.health()
			resp, err := structpb.NewStruct(map[string]any{
				"depth":       float64(depth),
				"capacity":    float64(q.capacity),
				"utilization": util,
			})
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}
			return connect.NewResponse(resp), nil
		},
	))

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
	}

	stopDrain := make(chan struct{})
	go q.runDrainLoop(stopDrain, consumeEvery)

	go func() {
		log.Printf("queue-stub listening on %s", addr)
		log.Printf("enqueue=%s health=%s", enqueueProcedure, healthProcedure)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("queue-stub server error: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	close(stopDrain)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("queue-stub shutdown error: %v", err)
	}
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

func (q *stubQueue) health() (depth int, util float64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.depth, q.utilizationLocked()
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
