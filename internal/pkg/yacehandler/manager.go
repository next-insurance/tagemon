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

package yacehandler

import (
	"github.com/next-insurance/tagemon-dev/internal/pkg/confighandler"
	"github.com/next-insurance/tagemon-dev/internal/pkg/tagshandler"
	ctrl "sigs.k8s.io/controller-runtime"
)

// SetupWithManager configures the YACE handler with the manager
func SetupWithManager(mgr ctrl.Manager, config *confighandler.Config, tagsHandlerInstance *tagshandler.Handler) error {
	reconciler := &Reconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		Config:      config,
		TagsHandler: tagsHandlerInstance,
	}

	return reconciler.SetupWithManager(mgr)
}
