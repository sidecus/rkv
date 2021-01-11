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

	fmt.Print("==================================\n")
	fmt.Printf("Benchmark with:\n%3d grpc connection\n%3d concurrent clients\n", 1, clients)
	fmt.Print("==================================\n")

	benchmarkSet(conn, times)
	benchmarkGet(conn, times)
}

func benchmarkSet(conn *grpc.ClientConn, total int) {
	duration, results := measureBatches(conn, total, clients, func(ctx context.Context, client pb.KVStoreRaftClient, i int) bool {
		r, err := client.Set(ctx, &pb.SetRequest{Key: fmt.Sprintf("k%d", i), Value: fmt.Sprint(i)})
		return err == nil && r.Success
	})

	var aggr batchResult
	for v := range results {
		aggr.total += v.total
		aggr.duration += v.duration
		aggr.successcnt += v.successcnt
	}

	printResult("Set", duration, aggr)
}

func benchmarkGet(conn *grpc.ClientConn, total int) {
	duration, results := measureBatches(conn, total, clients, func(ctx context.Context, client pb.KVStoreRaftClient, i int) bool {
		ret, err := client.Get(ctx, &pb.GetRequest{Key: fmt.Sprintf("k%d", i)})
		return err == nil && ret.Success && ret.Value == fmt.Sprint(i)
	})

	var aggr batchResult
	for v := range results {
		aggr.total += v.total
		aggr.duration += v.duration
		aggr.successcnt += v.successcnt
	}

	printResult("Get", duration, aggr)
}

func printResult(op string, totalDuration time.Duration, aggregated batchResult) {
	fmt.Printf("%s\n", op)
	fmt.Printf("  Total %s calls     : %d\n", op, aggregated.total)
	fmt.Printf("  Total %s success   : %d\n", op, aggregated.successcnt)
	fmt.Printf("  %s success rate    : %.2f%%\n", op, float64(aggregated.successcnt)*100/float64(aggregated.total))
	fmt.Printf("  Total time taken    : %dms\n", totalDuration/time.Millisecond)
	fmt.Printf("  Average %s latency : %dms\n", op, aggregated.duration/time.Millisecond/time.Duration(aggregated.total))
}

type batchResult struct {
	total      int
	successcnt int
	duration   time.Duration
}

type execFn func(ctx context.Context, client pb.KVStoreRaftClient, i int) bool

// run the rpc based execFn for "total" times in "batches".
// each batch is run in a separate goroutine
func measureBatches(conn *grpc.ClientConn, total int, batches int, fn execFn) (time.Duration, <-chan batchResult) {
	batchsize := (total + batches - 1) / batches

	results := make(chan batchResult, batches)
	defer close(results)

	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < batches; i++ {
		start, count := i*batchsize, batchsize
		if start > total {
			count = 0
		} else if count > total-start {
			count = total - start
		}

		wrapperFn := func() (result batchResult) {
			if count <= 0 {
				return
			}

			client := pb.NewKVStoreRaftClient(conn)
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			tstart := time.Now()
			for i := start; i < start+count; i++ {
				if fn(ctx, client, i) {
					result.successcnt++
				}
			}

			result.duration = time.Now().Sub(tstart)
			result.total = count
			return
		}

		wg.Add(1)
		go func() {
			results <- wrapperFn()
			wg.Done()
		}()
	}
	wg.Wait()
	duration := time.Now().Sub(start)

	return duration, results
}
