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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"

	v1alpha1 "github.com/next-insurance/tagemon-dev/api/v1alpha1"
	"github.com/next-insurance/tagemon-dev/internal/confighandler"
	"github.com/next-insurance/tagemon-dev/internal/tagshandler"
)

const (
	finalizerName = "tagemon.io/finalizer"
)

type Reconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	Config      *confighandler.Config
	TagsHandler *tagshandler.Handler
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Tagemon{}).
		Named("yace-handler").
		Complete(r)
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	var tagemon v1alpha1.Tagemon

	if err := r.Get(ctx, req.NamespacedName, &tagemon); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if tagemon.GetDeletionTimestamp() != nil {
		return r.handleDelete(ctx, &tagemon)
	}

	if !controllerutil.ContainsFinalizer(&tagemon, finalizerName) && tagemon.Status.ObservedGeneration == 0 {
		return r.handleCreate(ctx, &tagemon)
	}

	if tagemon.GetGeneration() != tagemon.Status.ObservedGeneration {
		return r.handleModify(ctx, &tagemon)
	}

	return ctrl.Result{}, nil
}

// =============================================================================
// LIFECYCLE HANDLERS
// =============================================================================

func (r *Reconciler) handleCreate(ctx context.Context, tagemon *v1alpha1.Tagemon) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	tagemon.Status.DeploymentID = uuid.New().String()

	yaceConfig, err := r.generateYACEConfig(tagemon)
	if err != nil {
		logger.Info("TAGEMON CREATE FAILED", "name", tagemon.Name, "error", err.Error())
		return ctrl.Result{}, fmt.Errorf("failed to generate YACE config: %w", err)
	}

	if err := r.createConfigMap(ctx, tagemon, yaceConfig); err != nil {
		logger.Info("TAGEMON CREATE FAILED", "name", tagemon.Name, "error", err.Error())
		return ctrl.Result{}, err
	}

	if err := r.createDeployment(ctx, tagemon); err != nil {
		logger.Info("TAGEMON CREATE FAILED", "name", tagemon.Name, "error", err.Error())
		return ctrl.Result{}, err
	}

	if result, err := r.updateStatus(ctx, tagemon); err != nil {
		return result, err
	}

	controllerutil.AddFinalizer(tagemon, finalizerName)
	if err := r.Update(ctx, tagemon); err != nil {
		if errors.IsConflict(err) {
			// Retry on conflict
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info("TAGEMON CREATE SUCCESS", "name", tagemon.Name, "type", tagemon.Spec.Type, "regions", tagemon.Spec.Regions)

	// Trigger tagshandler on create event
	if r.TagsHandler != nil {
		go func() {
			logger.Info("Triggering TagsHandler compliance check on CREATE", "name", tagemon.Name)
			if _, err := r.TagsHandler.CheckCompliance(context.Background(), r.Config.TagsHandler.Namespace, r.Config.TagsHandler.ViewARN, r.Config.TagsHandler.Region); err != nil {
				logger.Error(err, "TagsHandler compliance check failed on CREATE", "name", tagemon.Name)
			}
		}()
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) handleModify(ctx context.Context, tagemon *v1alpha1.Tagemon) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	configMapChanged, err := r.updateConfigMap(ctx, tagemon)
	if err != nil {
		logger.Info("TAGEMON MODIFY FAILED", "name", tagemon.Name, "error", err.Error())
		return ctrl.Result{}, err
	}

	err = r.annotateDeploymentIfRestartRequired(ctx, tagemon, configMapChanged)
	if err != nil {
		logger.Info("TAGEMON MODIFY FAILED", "name", tagemon.Name, "error", err.Error())
		return ctrl.Result{}, err
	}

	err = r.updateDeployment(ctx, tagemon)
	if err != nil {
		logger.Info("TAGEMON MODIFY FAILED", "name", tagemon.Name, "error", err.Error())
		return ctrl.Result{}, err
	}

	if result, err := r.updateStatus(ctx, tagemon); err != nil {
		return result, err
	}

	logger.Info("TAGEMON MODIFY SUCCESS", "name", tagemon.Name, "type", tagemon.Spec.Type, "regions", tagemon.Spec.Regions)

	// Trigger tagshandler on modify event
	if r.TagsHandler != nil {
		go func() {
			logger.Info("Triggering TagsHandler compliance check on MODIFY", "name", tagemon.Name)
			if _, err := r.TagsHandler.CheckCompliance(context.Background(), r.Config.TagsHandler.Namespace, r.Config.TagsHandler.ViewARN, r.Config.TagsHandler.Region); err != nil {
				logger.Error(err, "TagsHandler compliance check failed on MODIFY", "name", tagemon.Name)
			}
		}()
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) handleDelete(ctx context.Context, tagemon *v1alpha1.Tagemon) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if err := r.deleteConfigMap(ctx, tagemon); err != nil {
		logger.Info("TAGEMON DELETE FAILED", "name", tagemon.Name, "error", err.Error())
		return ctrl.Result{}, err
	}

	if err := r.deleteDeployment(ctx, tagemon); err != nil {
		logger.Info("TAGEMON DELETE FAILED", "name", tagemon.Name, "error", err.Error())
		return ctrl.Result{}, err
	}

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

// =============================================================================
// RESOURCE CREATION FUNCTIONS
// =============================================================================

func (r *Reconciler) createConfigMap(ctx context.Context, tagemon *v1alpha1.Tagemon, yaceConfig string) error {
	logger := log.FromContext(ctx)

	configMapName := r.buildResourceName(tagemon, "yace-cm")
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: tagemon.Namespace,
			Annotations: map[string]string{
				"tagemon.io/deployment-id": tagemon.Status.DeploymentID,
			},
		},
		Data: map[string]string{
			"config.yml": yaceConfig,
		},
	}

	if err := r.Create(ctx, configMap); err != nil {
		return fmt.Errorf("failed to create ConfigMap: %w", err)
	}
	logger.Info("ConfigMap created successfully", "name", configMap.Name, "deploymentID", tagemon.Status.DeploymentID)

	tagemon.Status.ConfigMapName = configMapName
	return nil
}

func (r *Reconciler) createDeployment(ctx context.Context, tagemon *v1alpha1.Tagemon) error {
	logger := log.FromContext(ctx)

	deploymentName := r.buildResourceName(tagemon, "yace")
	configMapName := tagemon.Status.ConfigMapName

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: tagemon.Namespace,
			Annotations: map[string]string{
				"tagemon.io/deployment-id": tagemon.Status.DeploymentID,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: func() *int32 { r := int32(1); return &r }(),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": deploymentName,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":                    deploymentName,
						"app.kubernetes.io/name": "tagemon-yace",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:      "yace",
							Image:     "prometheuscommunity/yet-another-cloudwatch-exporter:v0.62.1",
							Resources: r.buildResourceRequirements(tagemon.Spec.PodResources),
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 5000,
									Name:          "metrics",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/tmp/config.yml",
									SubPath:   "config.yml",
								},
							},
							Args: []string{
								"--config.file=/tmp/config.yml",
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: configMapName,
									},
									Items: []corev1.KeyToPath{
										{
											Key:  "config.yml",
											Path: "config.yml",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Only set ServiceAccountName if it's configured
	if r.Config.ServiceAccountName != "" {
		deployment.Spec.Template.Spec.ServiceAccountName = r.Config.ServiceAccountName
	}

	if err := r.Create(ctx, deployment); err != nil {
		return fmt.Errorf("failed to create Deployment: %w", err)
	}
	logger.Info("YACE Deployment created successfully", "name", deployment.Name, "deploymentID", tagemon.Status.DeploymentID)

	tagemon.Status.DeploymentName = deploymentName
	return nil
}

// =============================================================================
// RESOURCE UPDATE FUNCTIONS
// =============================================================================

func (r *Reconciler) updateConfigMap(ctx context.Context, tagemon *v1alpha1.Tagemon) (bool, error) {
	logger := log.FromContext(ctx)

	if tagemon.Status.ConfigMapName == "" {
		return false, nil
	}

	configMap := &corev1.ConfigMap{}
	configMapKey := client.ObjectKey{
		Namespace: tagemon.Namespace,
		Name:      tagemon.Status.ConfigMapName,
	}

	if err := r.Get(ctx, configMapKey, configMap); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("ConfigMap not found, may have been deleted", "name", tagemon.Status.ConfigMapName)
			return false, nil
		} else {
			logger.Error(err, "Failed to get ConfigMap", "name", tagemon.Status.ConfigMapName)
			return false, err
		}
	}

	yaceConfig, err := r.generateYACEConfig(tagemon)
	if err != nil {
		logger.Error(err, "Failed to generate YACE config for update")
		return false, err
	}

	if currentConfig, exists := configMap.Data["config.yml"]; !exists || currentConfig != yaceConfig {
		configMap.Data = map[string]string{
			"config.yml": yaceConfig,
		}
		if err := r.Update(ctx, configMap); err != nil {
			logger.Error(err, "Failed to update ConfigMap", "name", configMap.Name)
			return false, err
		}
		logger.Info("ConfigMap updated successfully", "name", configMap.Name)
		return true, nil
	}

	return false, nil
}

func (r *Reconciler) updateDeployment(ctx context.Context, tagemon *v1alpha1.Tagemon) error {
	logger := log.FromContext(ctx)
	if tagemon.Status.DeploymentName == "" {
		return nil
	}

	dep := &appsv1.Deployment{}
	key := client.ObjectKey{Namespace: tagemon.Namespace, Name: tagemon.Status.DeploymentName}
	if err := r.Get(ctx, key, dep); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Deployment not found, may have been deleted", "name", tagemon.Status.DeploymentName)
			return nil
		}
		return err
	}

	updated := dep.DeepCopy()

	r.applyPodResources(updated, tagemon)

	// If nothing changed, return early
	if reflect.DeepEqual(dep.Spec.Template.Spec, updated.Spec.Template.Spec) {
		return nil
	}

	// Persist changes
	dep.Spec.Template.Spec = updated.Spec.Template.Spec
	if err := r.Update(ctx, dep); err != nil {
		return err
	}
	logger.Info("Deployment spec updated from CRD", "name", dep.Name)
	return nil
}

func (r *Reconciler) annotateDeploymentIfRestartRequired(ctx context.Context, tagemon *v1alpha1.Tagemon, changeFlags ...bool) error {
	logger := log.FromContext(ctx)

	anyChanged := false
	for _, changed := range changeFlags {
		if changed {
			anyChanged = true
			break
		}
	}

	if anyChanged && tagemon.Status.DeploymentName != "" {
		deployment := &appsv1.Deployment{}
		deploymentKey := client.ObjectKey{
			Namespace: tagemon.Namespace,
			Name:      tagemon.Status.DeploymentName,
		}

		if err := r.Get(ctx, deploymentKey, deployment); err != nil {
			if errors.IsNotFound(err) {
				logger.Info("Deployment not found, may have been deleted", "name", tagemon.Status.DeploymentName)
			} else {
				logger.Error(err, "Failed to get Deployment", "name", tagemon.Status.DeploymentName)
				return err
			}
		} else {
			if deployment.Spec.Template.Annotations == nil {
				deployment.Spec.Template.Annotations = make(map[string]string)
			}
			deployment.Spec.Template.Annotations["tagemon.io/restartedAt"] = time.Now().Format(time.RFC3339)

			if err := r.Update(ctx, deployment); err != nil {
				logger.Error(err, "Failed to update Deployment for pod restart", "name", deployment.Name)
				return err
			}
			logger.Info("Deployment updated, restarting pods...", "name", deployment.Name, "anyChanged", anyChanged)
		}
	} else {
		logger.Info("No changes detected, pod restart not needed", "anyChanged", anyChanged)
	}

	return nil
}

// =============================================================================
// RESOURCE DELETION FUNCTIONS
// =============================================================================

func (r *Reconciler) deleteConfigMap(ctx context.Context, tagemon *v1alpha1.Tagemon) error {
	logger := log.FromContext(ctx)

	if tagemon.Status.ConfigMapName == "" {
		return nil
	}

	configMap := &corev1.ConfigMap{}
	configMapKey := client.ObjectKey{
		Namespace: tagemon.Namespace,
		Name:      tagemon.Status.ConfigMapName,
	}

	if err := r.Get(ctx, configMapKey, configMap); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("ConfigMap not found, may have been already deleted", "name", tagemon.Status.ConfigMapName)
			return nil
		} else {
			logger.Error(err, "Failed to get ConfigMap", "name", tagemon.Status.ConfigMapName)
			return err
		}
	}

	if err := r.Delete(ctx, configMap); err != nil {
		logger.Error(err, "Failed to delete ConfigMap", "name", configMap.Name)
		return err
	}
	logger.Info("ConfigMap deleted successfully", "name", configMap.Name)
	return nil
}

func (r *Reconciler) deleteDeployment(ctx context.Context, tagemon *v1alpha1.Tagemon) error {
	logger := log.FromContext(ctx)

	if tagemon.Status.DeploymentName == "" {
		return nil
	}

	deployment := &appsv1.Deployment{}
	deploymentKey := client.ObjectKey{
		Namespace: tagemon.Namespace,
		Name:      tagemon.Status.DeploymentName,
	}

	if err := r.Get(ctx, deploymentKey, deployment); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Deployment not found, may have been already deleted", "name", tagemon.Status.DeploymentName)
			return nil
		} else {
			logger.Error(err, "Failed to get Deployment", "name", tagemon.Status.DeploymentName)
			return err
		}
	}

	if err := r.Delete(ctx, deployment); err != nil {
		logger.Error(err, "Failed to delete Deployment", "name", deployment.Name)
		return err
	}
	logger.Info("Deployment deleted successfully", "name", deployment.Name)
	return nil
}

// =============================================================================
// UTILITY FUNCTIONS
// =============================================================================

func (r *Reconciler) updateStatus(ctx context.Context, tagemon *v1alpha1.Tagemon) (ctrl.Result, error) {
	tagemon.Status.ObservedGeneration = tagemon.GetGeneration()
	if err := r.Status().Update(ctx, tagemon); err != nil {
		if errors.IsConflict(err) {
			// Retry on conflict
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func setMetricOrGlobalValue(m map[string]interface{}, key string, metric *v1alpha1.TagemonMetric, spec *v1alpha1.TagemonSpec) {

	fieldName := strings.ToUpper(key[:1]) + key[1:]

	metricValue := reflect.ValueOf(metric).Elem().FieldByName(fieldName)
	if metricValue.IsValid() && !metricValue.IsZero() {
		if metricValue.Kind() == reflect.Ptr {
			m[key] = metricValue.Elem().Interface()
		} else {
			m[key] = metricValue.Interface()
		}
		return
	}

	specValue := reflect.ValueOf(spec).Elem().FieldByName(fieldName)
	if specValue.IsValid() && !specValue.IsZero() {
		if specValue.Kind() == reflect.Ptr {
			m[key] = specValue.Elem().Interface()
		} else {
			m[key] = specValue.Interface()
		}
	}
}

func (r *Reconciler) generateYACEConfig(tagemon *v1alpha1.Tagemon) (string, error) {
	config := map[string]interface{}{
		"apiVersion": "v1alpha1",
		"sts-region": "us-east-1", // Default STS region
		"discovery": map[string]interface{}{
			"exportedTagsOnMetrics": map[string]interface{}{
				tagemon.Spec.Type: []string{"Name"},
			},
			"jobs": []map[string]interface{}{
				{
					"type":    tagemon.Spec.Type,
					"regions": tagemon.Spec.Regions,
					"roles": func() []map[string]interface{} {
						roles := make([]map[string]interface{}, len(tagemon.Spec.Roles))
						for i, role := range tagemon.Spec.Roles {
							roleMap := map[string]interface{}{
								"roleArn": role.RoleArn,
							}
							if role.ExternalId != "" {
								roleMap["externalId"] = role.ExternalId
							}
							roles[i] = roleMap
						}
						return roles
					}(),
					"searchTags": func() []map[string]string {
						tags := make([]map[string]string, len(tagemon.Spec.SearchTags))
						for i, tag := range tagemon.Spec.SearchTags {
							tags[i] = map[string]string{
								"key":   tag.Key,
								"value": tag.Value,
							}
						}
						return tags
					}(),
					"metrics": func() []map[string]interface{} {
						metrics := make([]map[string]interface{}, len(tagemon.Spec.Metrics))
						for i, metric := range tagemon.Spec.Metrics {
							m := map[string]interface{}{
								"name": metric.Name,
							}

							setMetricOrGlobalValue(m, "statistics", &metric, &tagemon.Spec)
							setMetricOrGlobalValue(m, "period", &metric, &tagemon.Spec)
							setMetricOrGlobalValue(m, "length", &metric, &tagemon.Spec)
							setMetricOrGlobalValue(m, "nilToZero", &metric, &tagemon.Spec)
							setMetricOrGlobalValue(m, "addCloudwatchTimestamp", &metric, &tagemon.Spec)

							metrics[i] = m
						}
						return metrics
					}(),
				},
			},
		},
	}

	yamlData, err := yaml.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal YACE config: %w", err)
	}

	return string(yamlData), nil
}

func (r *Reconciler) applyPodResources(deployment *appsv1.Deployment, tagemon *v1alpha1.Tagemon) {
	if len(deployment.Spec.Template.Spec.Containers) == 0 {
		return
	}
	deployment.Spec.Template.Spec.Containers[0].Resources = r.buildResourceRequirements(tagemon.Spec.PodResources)
}

func (r *Reconciler) buildResourceRequirements(podRes *v1alpha1.PodResources) corev1.ResourceRequirements {
	res := corev1.ResourceRequirements{}
	if podRes == nil {
		return res
	}
	if podRes.Requests != nil {
		res.Requests = podRes.Requests
	}
	if podRes.Limits != nil {
		res.Limits = podRes.Limits
	}
	return res
}

func (r *Reconciler) buildResourceName(tagemon *v1alpha1.Tagemon, suffix string) string {
	serviceType := strings.ToLower(strings.TrimPrefix(tagemon.Spec.Type, "AWS/"))

	shortHash := generateShortHash(fmt.Sprintf("%s-%s", tagemon.Namespace, tagemon.Name))

	baseName := fmt.Sprintf("%s-%s-%s", serviceType, suffix, shortHash)

	if tagemon.Spec.NamePrefix != "" {
		return fmt.Sprintf("%s-%s", tagemon.Spec.NamePrefix, baseName)
	}

	return baseName
}

func generateShortHash(input string) string {
	hash := sha256.Sum256([]byte(input))

	return hex.EncodeToString(hash[:])[:5]
}
