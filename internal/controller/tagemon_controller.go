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
	"reflect"
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
)

const (
	finalizerName = "tagemon.io/finalizer"
)

type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Tagemon{}).
		Named("tagemon").
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

	if err := r.createServiceAccount(ctx, tagemon); err != nil {
		logger.Info("TAGEMON CREATE FAILED", "name", tagemon.Name, "error", err.Error())
		return ctrl.Result{}, err
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
	return ctrl.Result{}, nil
}

func (r *Reconciler) handleModify(ctx context.Context, tagemon *v1alpha1.Tagemon) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	configMapChanged := false
	serviceAccountChanged := false

	configMapChanged, err := r.updateConfigMap(ctx, tagemon)
	if err != nil {
		logger.Info("TAGEMON MODIFY FAILED", "name", tagemon.Name, "error", err.Error())
		return ctrl.Result{}, err
	}

	serviceAccountChanged, err = r.updateServiceAccount(ctx, tagemon)
	if err != nil {
		logger.Info("TAGEMON MODIFY FAILED", "name", tagemon.Name, "error", err.Error())
		return ctrl.Result{}, err
	}

	err = r.annotateDeploymentIfRestartRequired(ctx, tagemon, configMapChanged, serviceAccountChanged)
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

	if err := r.deleteServiceAccount(ctx, tagemon); err != nil {
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

func (r *Reconciler) createServiceAccount(ctx context.Context, tagemon *v1alpha1.Tagemon) error {
	logger := log.FromContext(ctx)

	serviceAccountName := fmt.Sprintf("%s-yace", tagemon.Name)
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountName,
			Namespace: tagemon.Namespace,
			Annotations: map[string]string{
				"eks.amazonaws.com/role-arn": tagemon.Spec.ServiceAccountRoleArn,
				"tagemon.io/deployment-id":   tagemon.Status.DeploymentID,
			},
		},
	}

	if err := r.Create(ctx, serviceAccount); err != nil {
		return fmt.Errorf("failed to create ServiceAccount: %w", err)
	}
	logger.Info("ServiceAccount created successfully", "name", serviceAccount.Name, "roleArn", tagemon.Spec.ServiceAccountRoleArn, "deploymentID", tagemon.Status.DeploymentID)

	tagemon.Status.ServiceAccountName = serviceAccountName
	return nil
}

func (r *Reconciler) createConfigMap(ctx context.Context, tagemon *v1alpha1.Tagemon, yaceConfig string) error {
	logger := log.FromContext(ctx)

	configMapName := tagemon.Name
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

	deploymentName := fmt.Sprintf("%s-yace", tagemon.Name)
	serviceAccountName := tagemon.Status.ServiceAccountName
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
						"app": deploymentName,
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: serviceAccountName,
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

func (r *Reconciler) updateServiceAccount(ctx context.Context, tagemon *v1alpha1.Tagemon) (bool, error) {
	logger := log.FromContext(ctx)

	if tagemon.Status.ServiceAccountName == "" {
		return false, nil
	}

	serviceAccount := &corev1.ServiceAccount{}
	serviceAccountKey := client.ObjectKey{
		Namespace: tagemon.Namespace,
		Name:      tagemon.Status.ServiceAccountName,
	}

	if err := r.Get(ctx, serviceAccountKey, serviceAccount); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("ServiceAccount not found, may have been deleted", "name", tagemon.Status.ServiceAccountName)
			return false, nil
		} else {
			logger.Error(err, "Failed to get ServiceAccount", "name", tagemon.Status.ServiceAccountName)
			return false, err
		}
	}

	// Check if the role ARN actually changed
	currentRoleArn := ""
	if serviceAccount.Annotations != nil {
		currentRoleArn = serviceAccount.Annotations["eks.amazonaws.com/role-arn"]
	}

	if currentRoleArn != tagemon.Spec.ServiceAccountRoleArn {
		if serviceAccount.Annotations == nil {
			serviceAccount.Annotations = make(map[string]string)
		}
		serviceAccount.Annotations["eks.amazonaws.com/role-arn"] = tagemon.Spec.ServiceAccountRoleArn

		if err := r.Update(ctx, serviceAccount); err != nil {
			logger.Error(err, "Failed to update ServiceAccount", "name", serviceAccount.Name)
			return false, err
		}
		logger.Info("ServiceAccount updated successfully", "name", serviceAccount.Name, "roleArn", tagemon.Spec.ServiceAccountRoleArn)
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

func (r *Reconciler) deleteServiceAccount(ctx context.Context, tagemon *v1alpha1.Tagemon) error {
	logger := log.FromContext(ctx)

	if tagemon.Status.ServiceAccountName == "" {
		return nil
	}

	serviceAccount := &corev1.ServiceAccount{}
	serviceAccountKey := client.ObjectKey{
		Namespace: tagemon.Namespace,
		Name:      tagemon.Status.ServiceAccountName,
	}

	if err := r.Get(ctx, serviceAccountKey, serviceAccount); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("ServiceAccount not found, may have been already deleted", "name", tagemon.Status.ServiceAccountName)
			return nil
		} else {
			logger.Error(err, "Failed to get ServiceAccount", "name", tagemon.Status.ServiceAccountName)
			return err
		}
	}

	if err := r.Delete(ctx, serviceAccount); err != nil {
		logger.Error(err, "Failed to delete ServiceAccount", "name", serviceAccount.Name)
		return err
	}
	logger.Info("ServiceAccount deleted successfully", "name", serviceAccount.Name)
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
func (r *Reconciler) generateYACEConfig(tagemon *v1alpha1.Tagemon) (string, error) {
	config := map[string]interface{}{
		"apiVersion": "v1alpha1",
		"sts-region": "us-east-1", // Default STS region
		"discovery": map[string]interface{}{
			"exportedTagsOnMetrics": map[string]interface{}{
				tagemon.Spec.Type: tagemon.Spec.ExportedTagsOnMetrics,
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

							if len(metric.Statistics) > 0 {
								m["statistics"] = metric.Statistics
							} else if len(tagemon.Spec.Statistics) > 0 {
								m["statistics"] = tagemon.Spec.Statistics
							}

							if metric.Period != nil {
								m["period"] = *metric.Period
							} else if tagemon.Spec.Period != nil {
								m["period"] = *tagemon.Spec.Period
							}

							if metric.Length != nil {
								m["length"] = *metric.Length
							} else if tagemon.Spec.Length != nil {
								m["length"] = *tagemon.Spec.Length
							}

							if metric.NilToZero != nil {
								m["nilToZero"] = *metric.NilToZero
							} else if tagemon.Spec.NilToZero != nil {
								m["nilToZero"] = *tagemon.Spec.NilToZero
							}

							if metric.AddCloudwatchTimestamp != nil {
								m["addCloudwatchTimestamp"] = *metric.AddCloudwatchTimestamp
							} else if tagemon.Spec.AddCloudwatchTimestamp != nil {
								m["addCloudwatchTimestamp"] = *tagemon.Spec.AddCloudwatchTimestamp
							}

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
