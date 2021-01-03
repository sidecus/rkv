package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/sidecus/raft/pkg/rkv/pb"
	"google.golang.org/grpc"
)

const clients int = 100

func benchmark(conn *grpc.ClientConn, times int) {
	if times <= 0 {
		fmt.Println("times for benchmark mode cannot be 0 or less")
		os.Exit(1)
	}

	fmt.Print("=====================================================\n")
	fmt.Printf("Benchmark with 1 connection, %d concurrent clients:\n", clients)

	benchmarkSet(conn, times)
	benchmarkGet(conn, times)
}

func benchmarkGet(conn *grpc.ClientConn, total int) {
	duration, results := batch(total, clients, func(start int, count int) (result batchResult) {
		if count <= 0 {
			return
		}

		result.total = count
		client := pb.NewKVStoreRaftClient(conn)
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		tstart := time.Now()
		for i := start; i < start+count; i++ {
			ret, err := client.Get(ctx, &pb.GetRequest{Key: fmt.Sprintf("k%d", i)})
			if err == nil && ret.Success && ret.Value == fmt.Sprint(i) {
				result.successcnt++
			} else {
				fmt.Printf("Set didn't succeed for k%d\n", i)
			}
		}
		result.duration = time.Now().Sub(tstart)
		return
	})

	var final batchResult
	for v := range results {
		final.total += v.total
		final.duration += v.duration
		final.successcnt += v.successcnt
	}

	fmt.Print("GET\n")
	fmt.Printf("  Total get calls     : %d\n", final.total)
	fmt.Printf("  Total get success   : %d\n", final.successcnt)
	fmt.Printf("  Total time taken    : %dms\n", duration/time.Millisecond)
	fmt.Printf("  Average get latency : %dms\n", final.duration/time.Millisecond/time.Duration(total))
}

func benchmarkSet(conn *grpc.ClientConn, total int) {
	duration, results := batch(total, clients, func(start int, count int) (result batchResult) {
		if count <= 0 {
			return
		}

		result.total = count

		client := pb.NewKVStoreRaftClient(conn)
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		tstart := time.Now()
		for i := start; i < start+count; i++ {
			r, err := client.Set(ctx, &pb.SetRequest{Key: fmt.Sprintf("k%d", i), Value: fmt.Sprint(i)})
			if err == nil && r.Success {
				result.successcnt++
			}
		}
		result.duration = time.Now().Sub(tstart)
		return
	})

	var final batchResult
	for v := range results {
		final.total += v.total
		final.duration += v.duration
		final.successcnt += v.successcnt
	}

	fmt.Print("SET\n")
	fmt.Printf("  Total set clients   : %d\n", clients)
	fmt.Printf("  Total set calls     : %d\n", final.total)
	fmt.Printf("  Total set success   : %d\n", final.successcnt)
	fmt.Printf("  Total time taken    : %d ms\n", duration/time.Millisecond)
	fmt.Printf("  Average set latency : %d ms\n", final.duration*1.0/time.Millisecond/time.Duration(final.total))
}

type batchResult struct {
	total      int
	successcnt int
	duration   time.Duration
}

type batchFn func(start int, count int) batchResult

// run something for "total" times in "batches".
// each batch is run in a separate go routine
func batch(total int, batches int, fn batchFn) (time.Duration, <-chan batchResult) {
	batchsize := (total + batches - 1) / batches

	results := make(chan batchResult, batches)
	defer close(results)

	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < batches; i++ {
		wg.Add(1)
		start, count := i*batchsize, batchsize
		if start > total {
			count = 0
		} else if count > total-start {
			count = total - start
		}

		go func() {
			results <- fn(start, count)
			wg.Done()
		}()
	}
	wg.Wait()
	duration := time.Now().Sub(start)

	return duration, results
}
