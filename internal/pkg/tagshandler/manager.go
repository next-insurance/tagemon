/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tagshandler

import (
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/next-insurance/tagemon-dev/internal/pkg/confighandler"
)

// SetupWithManager configures the tags handler runner with the manager
func SetupWithManager(mgr ctrl.Manager, config *confighandler.Config, handler *Handler) error {
	// Only set up if ViewARN is configured and handler is provided
	if config.TagsHandler.ViewARN == "" || handler == nil {
		return nil
	}

	// Use configured interval, default to 30 minutes if not set
	interval := 30 * time.Minute
	if config.TagsHandler.Interval != nil {
		interval = *config.TagsHandler.Interval
	}

	// Create and add scheduler
	scheduler := &Scheduler{
		mgr:      mgr,
		handler:  handler,
		config:   config,
		interval: interval,
	}

	return mgr.Add(scheduler)
}
