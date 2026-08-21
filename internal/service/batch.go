package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/auth"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/clearance"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

type BatchDeclarationResult struct {
	Index    int
	VoyageID string
	Voyage   *clearance.Voyage
	Error    error
}

func (coordinator Coordinator) DeclareBatch(ctx context.Context, actor auth.User, commands []DeclareVoyageCommand, concurrency int) ([]BatchDeclarationResult, error) {
	if err := coordinator.validate(); err != nil {
		return nil, err
	}
	if err := auth.Authorize(actor, actor.TenantID, auth.PermissionVoyageDeclare); err != nil {
		return nil, err
	}
	if len(commands) == 0 || len(commands) > 100 {
		return nil, fmt.Errorf("%w: declaration batch size", domain.ErrInvalid)
	}
	if concurrency <= 0 {
		concurrency = 4
	}
	if concurrency > 16 {
		concurrency = 16
	}
	seen := make(map[string]struct{}, len(commands))
	for _, command := range commands {
		if command.ID == "" {
			return nil, fmt.Errorf("%w: voyage id", domain.ErrInvalid)
		}
		if _, duplicate := seen[command.ID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate voyage %s", domain.ErrConflict, command.ID)
		}
		seen[command.ID] = struct{}{}
	}
	results := make([]BatchDeclarationResult, len(commands))
	jobs := make(chan int)
	var workers sync.WaitGroup
	for workerIndex := 0; workerIndex < concurrency; workerIndex++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				command := commands[index]
				result := BatchDeclarationResult{Index: index, VoyageID: command.ID}
				voyage, err := coordinator.Declare(ctx, actor, command)
				if err != nil {
					result.Error = err
				} else {
					copyVoyage := voyage
					result.Voyage = &copyVoyage
				}
				results[index] = result
			}
		}()
	}
	for index := range commands {
		select {
		case <-ctx.Done():
			close(jobs)
			workers.Wait()
			for remaining := index; remaining < len(results); remaining++ {
				results[remaining] = BatchDeclarationResult{Index: remaining, VoyageID: commands[remaining].ID, Error: ctx.Err()}
			}
			return results, ctx.Err()
		case jobs <- index:
		}
	}
	close(jobs)
	workers.Wait()
	return results, nil
}

func SuccessfulDeclarations(results []BatchDeclarationResult) []clearance.Voyage {
	voyages := make([]clearance.Voyage, 0, len(results))
	for _, result := range results {
		if result.Error == nil && result.Voyage != nil {
			voyages = append(voyages, *result.Voyage)
		}
	}
	return voyages
}
