// Copyright © 2016 Asteris, LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package exec

import (
	"sync"

	"golang.org/x/net/context"

	"github.com/asteris-llc/converge/load"
	"github.com/asteris-llc/converge/resource"
)

// ApplyWithStatus applies the operations checked in plan
func ApplyWithStatus(ctx context.Context, graph *load.Graph, plan []*PlanResult, status chan<- *StatusMessage) (results []*ApplyResult, err error) {
	// transform checks into something we can look up easily
	planResults := map[string]*PlanResult{}
	for _, result := range plan {
		planResults[result.Path] = result
	}

	// iterate over the checks, looking for things that need changes
	lock := new(sync.Mutex)

	err = graph.Walk(func(path string, res resource.Resource) error {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		planResult, ok := planResults[path]
		if !ok || !planResult.WillChange {
			return nil
		}

		task, ok := res.(resource.Task)
		if !ok {
			return nil
		}

		var (
			err    error
			result = &ApplyResult{
				Path:      path,
				OldStatus: planResult.CurrentStatus,
			}
		)

		select {
		case <-ctx.Done():
			return nil
		case status <- &StatusMessage{Path: path, Status: "applying"}:
		}

		result.NewStatus, result.Success, err = task.Apply()
		if err != nil {
			return err
		}

		lock.Lock()
		defer lock.Unlock()
		results = append(results, result)

		return nil
	})

	return results, err
}

// Apply does the same thing as ApplyWithStatus, but drops all status messages
func Apply(ctx context.Context, graph *load.Graph, plan []*PlanResult) (results []*ApplyResult, err error) {
	status := make(chan *StatusMessage, 1)
	defer close(status)
	go func() {
		for range status {
			continue
		}
	}()

	return ApplyWithStatus(ctx, graph, plan, status)
}
