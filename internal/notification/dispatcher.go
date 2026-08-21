package notification

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

type Sender interface {
	Send(context.Context, Delivery, Event) error
}

type DispatchResult struct {
	Delivery Delivery
	Sent     bool
	Error    error
}

type Dispatcher struct {
	Senders     map[Channel]Sender
	Concurrency int
}

func (dispatcher Dispatcher) Dispatch(ctx context.Context, event Event, deliveries []Delivery) ([]DispatchResult, error) {
	if len(deliveries) == 0 {
		return []DispatchResult{}, nil
	}
	concurrency := dispatcher.Concurrency
	if concurrency <= 0 {
		concurrency = 4
	}
	if concurrency > 32 {
		concurrency = 32
	}
	results := make([]DispatchResult, len(deliveries))
	jobs := make(chan int)
	var group sync.WaitGroup
	for workerIndex := 0; workerIndex < concurrency; workerIndex++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for index := range jobs {
				delivery := deliveries[index]
				result := DispatchResult{Delivery: delivery}
				sender := dispatcher.Senders[delivery.Channel]
				if sender == nil {
					result.Error = fmt.Errorf("no sender for channel %s", delivery.Channel)
				} else if err := sender.Send(ctx, delivery, event); err != nil {
					result.Error = err
				} else {
					result.Sent = true
				}
				results[index] = result
			}
		}()
	}
	for index := range deliveries {
		select {
		case jobs <- index:
		case <-ctx.Done():
			close(jobs)
			group.Wait()
			for remaining := index; remaining < len(deliveries); remaining++ {
				results[remaining] = DispatchResult{Delivery: deliveries[remaining], Error: ctx.Err()}
			}
			return results, ctx.Err()
		}
	}
	close(jobs)
	group.Wait()
	return results, nil
}

func Failed(results []DispatchResult) []DispatchResult {
	failed := make([]DispatchResult, 0)
	for _, result := range results {
		if !result.Sent || result.Error != nil {
			failed = append(failed, result)
		}
	}
	sort.Slice(failed, func(i, j int) bool { return failed[i].Delivery.ID < failed[j].Delivery.ID })
	return failed
}

func JoinErrors(results []DispatchResult) error {
	errorsToJoin := make([]error, 0)
	for _, result := range results {
		if result.Error != nil {
			errorsToJoin = append(errorsToJoin, fmt.Errorf("delivery %s: %w", result.Delivery.ID, result.Error))
		}
	}
	return errors.Join(errorsToJoin...)
}
