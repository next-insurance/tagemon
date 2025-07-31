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

package controller

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha1 "github.com/next-insurance/tagemon-dev/api/v1alpha1"
)

const (
	finalizerName = "tagemon.io/finalizer"
)

// Reconciler reconciles a Tagemon object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	// Fetch the Tagemon instance
	var tagemon v1alpha1.Tagemon

	// Check for errors in getting the Tagemon instance
	if err := r.Get(ctx, req.NamespacedName, &tagemon); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion
	if tagemon.GetDeletionTimestamp() != nil {
		return r.handleDelete(ctx, &tagemon)
	}

	// Handle creation (new resource)
	if r.isNewResource(&tagemon) {
		return r.handleCreate(ctx, &tagemon)
	}

	// Handle modification
	if r.isModified(&tagemon) {
		return r.handleModify(ctx, &tagemon)
	}

	return ctrl.Result{}, nil
}

// isNewResource checks if this is a newly created resource
func (r *Reconciler) isNewResource(tagemon *v1alpha1.Tagemon) bool {
	return !controllerutil.ContainsFinalizer(tagemon, finalizerName) && tagemon.Status.ObservedGeneration == 0
}

// isModified checks if the resource has been modified
func (r *Reconciler) isModified(tagemon *v1alpha1.Tagemon) bool {
	return tagemon.GetGeneration() != tagemon.Status.ObservedGeneration
}

// handleCreate processes a new Tagemon resource
func (r *Reconciler) handleCreate(ctx context.Context, tagemon *v1alpha1.Tagemon) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Execute create logic
	if err := r.executeCreate(tagemon); err != nil {
		logger.Info("TAGEMON CREATE FAILED", "name", tagemon.Name, "error", err.Error())
		return ctrl.Result{}, err
	}

	// Add finalizer and update status
	controllerutil.AddFinalizer(tagemon, finalizerName)
	if err := r.Update(ctx, tagemon); err != nil {
		if errors.IsConflict(err) {
			// Retry on conflict
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	tagemon.Status.ObservedGeneration = tagemon.GetGeneration()
	if err := r.Status().Update(ctx, tagemon); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("TAGEMON CREATE SUCCESS", "name", tagemon.Name, "type", tagemon.Spec.Type, "regions", tagemon.Spec.Regions)
	return ctrl.Result{}, nil
}

// handleModify processes changes to an existing Tagemon resource
func (r *Reconciler) handleModify(ctx context.Context, tagemon *v1alpha1.Tagemon) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Execute modify logic
	if err := r.executeModify(tagemon); err != nil {
		logger.Info("TAGEMON MODIFY FAILED", "name", tagemon.Name, "error", err.Error())
		return ctrl.Result{}, err
	}

	// Update status
	tagemon.Status.ObservedGeneration = tagemon.GetGeneration()
	if err := r.Status().Update(ctx, tagemon); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("TAGEMON MODIFY SUCCESS", "name", tagemon.Name, "type", tagemon.Spec.Type, "regions", tagemon.Spec.Regions)
	return ctrl.Result{}, nil
}

// handleDelete processes deletion of a Tagemon resource
func (r *Reconciler) handleDelete(ctx context.Context, tagemon *v1alpha1.Tagemon) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Execute delete logic
	if err := r.executeDelete(tagemon); err != nil {
		logger.Info("TAGEMON DELETE FAILED", "name", tagemon.Name, "error", err.Error())
		return ctrl.Result{}, err
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(tagemon, finalizerName)
	if err := r.Update(ctx, tagemon); err != nil {
		if errors.IsConflict(err) {
			// Retry on conflict
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info("TAGEMON DELETE SUCCESS", "name", tagemon.Name)
	return ctrl.Result{}, nil
}

// executeCreate contains the business logic for creating a Tagemon
func (r *Reconciler) executeCreate(tagemon *v1alpha1.Tagemon) error {
	// Basic validation
	if tagemon.Spec.Type == "" {
		return fmt.Errorf("type field is required")
	}
	if len(tagemon.Spec.Regions) == 0 {
		return fmt.Errorf("at least one region must be specified")
	}

	// Simulate some work - should be replaced with actual logic
	time.Sleep(100 * time.Millisecond)

	return nil
}

// executeModify contains the business logic for modifying a Tagemon
func (r *Reconciler) executeModify(tagemon *v1alpha1.Tagemon) error {
	// Basic validation
	if tagemon.Spec.Type == "" {
		return fmt.Errorf("type field is required")
	}
	if len(tagemon.Spec.Regions) == 0 {
		return fmt.Errorf("at least one region must be specified")
	}

	// Simulate some work - should be replaced with actual logic
	time.Sleep(100 * time.Millisecond)

	return nil
}

// executeDelete contains the business logic for deleting a Tagemon
func (r *Reconciler) executeDelete(tagemon *v1alpha1.Tagemon) error {
	// Basic validation (to avoid unparam error and use the parameter)
	if tagemon.Spec.Type == "" {
		return fmt.Errorf("type field is required for deletion")
	}

	// Simulate some work - should be replaced with actual logic
	time.Sleep(100 * time.Millisecond)

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Tagemon{}).
		Named("tagemon").
		Complete(r)
}
