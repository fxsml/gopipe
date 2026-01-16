package pipe

import (
	"context"
	"testing"
	"time"
)

// BenchmarkProcessing_StaticConcurrency benchmarks processing with static worker count.
func BenchmarkProcessing_StaticConcurrency(b *testing.B) {
	benchmarks := []struct {
		name        string
		concurrency int
		items       int
		workTime    time.Duration
	}{
		{"Concurrency1_Items100_Fast", 1, 100, 0},
		{"Concurrency4_Items100_Fast", 4, 100, 0},
		{"Concurrency8_Items100_Fast", 8, 100, 0},
		{"Concurrency1_Items100_Slow", 1, 100, time.Microsecond},
		{"Concurrency4_Items100_Slow", 4, 100, time.Microsecond},
		{"Concurrency8_Items100_Slow", 8, 100, time.Microsecond},
		{"Concurrency4_Items1000_Fast", 4, 1000, 0},
		{"Concurrency8_Items1000_Fast", 8, 1000, 0},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			workTime := bm.workTime
			process := func(ctx context.Context, v int) ([]int, error) {
				if workTime > 0 {
					time.Sleep(workTime)
				}
				return []int{v * 2}, nil
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				in := make(chan int, bm.items)
				out := startProcessing(context.Background(), in, process, Config{
					Concurrency: bm.concurrency,
					BufferSize:  bm.items,
				})

				// Send all items
				go func() {
					for j := 0; j < bm.items; j++ {
						in <- j
					}
					close(in)
				}()

				// Drain output
				for range out {
				}
			}
		})
	}
}

// BenchmarkProcessing_Autoscale benchmarks processing with autoscaling workers.
func BenchmarkProcessing_Autoscale(b *testing.B) {
	benchmarks := []struct {
		name       string
		minWorkers int
		maxWorkers int
		items      int
		workTime   time.Duration
	}{
		{"Min1Max4_Items100_Fast", 1, 4, 100, 0},
		{"Min1Max8_Items100_Fast", 1, 8, 100, 0},
		{"Min2Max8_Items100_Fast", 2, 8, 100, 0},
		{"Min1Max4_Items100_Slow", 1, 4, 100, time.Microsecond},
		{"Min1Max8_Items100_Slow", 1, 8, 100, time.Microsecond},
		{"Min2Max8_Items100_Slow", 2, 8, 100, time.Microsecond},
		{"Min2Max8_Items1000_Fast", 2, 8, 1000, 0},
		{"Min4Max16_Items1000_Fast", 4, 16, 1000, 0},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			workTime := bm.workTime
			process := func(ctx context.Context, v int) ([]int, error) {
				if workTime > 0 {
					time.Sleep(workTime)
				}
				return []int{v * 2}, nil
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				in := make(chan int, bm.items)
				out := startProcessing(context.Background(), in, process, Config{
					Autoscale: &AutoscaleConfig{
						MinWorkers:        bm.minWorkers,
						MaxWorkers:        bm.maxWorkers,
						ScaleUpCooldown:   time.Millisecond,
						ScaleDownCooldown: time.Millisecond,
						ScaleDownAfter:    100 * time.Millisecond,
						CheckInterval:     time.Millisecond,
					},
					BufferSize: bm.items,
				})

				// Send all items
				go func() {
					for j := 0; j < bm.items; j++ {
						in <- j
					}
					close(in)
				}()

				// Drain output
				for range out {
				}
			}
		})
	}
}

// BenchmarkProcessing_Comparison directly compares static vs autoscale for same workload.
func BenchmarkProcessing_Comparison(b *testing.B) {
	items := 500
	workTime := 10 * time.Microsecond

	process := func(ctx context.Context, v int) ([]int, error) {
		time.Sleep(workTime)
		return []int{v * 2}, nil
	}

	b.Run("Static_Concurrency4", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			in := make(chan int, items)
			out := startProcessing(context.Background(), in, process, Config{
				Concurrency: 4,
				BufferSize:  items,
			})

			go func() {
				for j := 0; j < items; j++ {
					in <- j
				}
				close(in)
			}()

			for range out {
			}
		}
	})

	b.Run("Static_Concurrency8", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			in := make(chan int, items)
			out := startProcessing(context.Background(), in, process, Config{
				Concurrency: 8,
				BufferSize:  items,
			})

			go func() {
				for j := 0; j < items; j++ {
					in <- j
				}
				close(in)
			}()

			for range out {
			}
		}
	})

	b.Run("Autoscale_Min1Max8", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			in := make(chan int, items)
			out := startProcessing(context.Background(), in, process, Config{
				Autoscale: &AutoscaleConfig{
					MinWorkers:        1,
					MaxWorkers:        8,
					ScaleUpCooldown:   time.Millisecond,
					ScaleDownCooldown: time.Millisecond,
					ScaleDownAfter:    50 * time.Millisecond,
					CheckInterval:     time.Millisecond,
				},
				BufferSize: items,
			})

			go func() {
				for j := 0; j < items; j++ {
					in <- j
				}
				close(in)
			}()

			for range out {
			}
		}
	})

	b.Run("Autoscale_Min4Max8", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			in := make(chan int, items)
			out := startProcessing(context.Background(), in, process, Config{
				Autoscale: &AutoscaleConfig{
					MinWorkers:        4,
					MaxWorkers:        8,
					ScaleUpCooldown:   time.Millisecond,
					ScaleDownCooldown: time.Millisecond,
					ScaleDownAfter:    50 * time.Millisecond,
					CheckInterval:     time.Millisecond,
				},
				BufferSize: items,
			})

			go func() {
				for j := 0; j < items; j++ {
					in <- j
				}
				close(in)
			}()

			for range out {
			}
		}
	})
}

// BenchmarkProcessing_BurstLoad simulates burst load patterns.
func BenchmarkProcessing_BurstLoad(b *testing.B) {
	burstSize := 100
	bursts := 5
	workTime := 50 * time.Microsecond

	process := func(ctx context.Context, v int) ([]int, error) {
		time.Sleep(workTime)
		return []int{v * 2}, nil
	}

	b.Run("Static_Concurrency4", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			in := make(chan int, burstSize)
			out := startProcessing(context.Background(), in, process, Config{
				Concurrency: 4,
				BufferSize:  burstSize,
			})

			go func() {
				for burst := 0; burst < bursts; burst++ {
					// Send burst
					for j := 0; j < burstSize; j++ {
						in <- j
					}
					// Brief pause between bursts
					time.Sleep(time.Millisecond)
				}
				close(in)
			}()

			for range out {
			}
		}
	})

	b.Run("Autoscale_Min1Max8", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			in := make(chan int, burstSize)
			out := startProcessing(context.Background(), in, process, Config{
				Autoscale: &AutoscaleConfig{
					MinWorkers:        1,
					MaxWorkers:        8,
					ScaleUpCooldown:   time.Millisecond,
					ScaleDownCooldown: 5 * time.Millisecond,
					ScaleDownAfter:    10 * time.Millisecond,
					CheckInterval:     time.Millisecond,
				},
				BufferSize: burstSize,
			})

			go func() {
				for burst := 0; burst < bursts; burst++ {
					// Send burst
					for j := 0; j < burstSize; j++ {
						in <- j
					}
					// Brief pause between bursts
					time.Sleep(time.Millisecond)
				}
				close(in)
			}()

			for range out {
			}
		}
	})
}
