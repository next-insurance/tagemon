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
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1alpha1 "github.com/next-insurance/tagemon-dev/api/v1alpha1"
	"github.com/next-insurance/tagemon-dev/internal/pkg/confighandler"
)

// =============================================================================
// UTILITY FUNCTION TESTS
// =============================================================================

func TestReconciler_Initialization(t *testing.T) {
	t.Run("should initialize reconciler correctly", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		config := &confighandler.Config{ServiceAccountName: "test-sa"}

		reconciler := &Reconciler{
			Client:      fakeClient,
			Scheme:      scheme,
			Config:      config,
			TagsHandler: nil, // No tagshandler needed for this test
		}

		if reconciler.Client == nil {
			t.Error("expected Client to be set")
		}
		if reconciler.Scheme == nil {
			t.Error("expected Scheme to be set")
		}
		if reconciler.Config == nil {
			t.Error("expected Config to be set")
		}
		if reconciler.Config.ServiceAccountName != "test-sa" {
			t.Errorf("expected ServiceAccountName to be 'test-sa', got %s", reconciler.Config.ServiceAccountName)
		}
	})
}

func TestGenerateYACEConfig(t *testing.T) {
	reconciler := &Reconciler{
		TagsHandler: nil, // No tagshandler needed for this test
	}

	t.Run("should generate valid YACE config with minimal spec", func(t *testing.T) {
		tagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-tagemon",
				Namespace: "default",
			},
			Spec: v1alpha1.TagemonSpec{
				Type:    "AWS/S3",
				Regions: []v1alpha1.AWSRegion{"us-east-1"},
				Roles: []v1alpha1.AWSRole{
					{RoleArn: "arn:aws:iam::123456789012:role/test-role"},
				},
				Metrics: []v1alpha1.TagemonMetric{
					{Name: "BucketSizeBytes"},
				},
			},
		}

		config, err := reconciler.generateYACEConfig(tagemon)
		if err != nil {
			t.Fatalf("generateYACEConfig failed: %v", err)
		}

		expectedStrings := []string{
			"apiVersion: v1alpha1",
			"sts-region: us-east-1",
			"discovery:",
			"exportedTagsOnMetrics:",
			"jobs:",
			"type: AWS/S3",
			"regions:",
			"- us-east-1",
			"roles:",
			"- roleArn: arn:aws:iam::123456789012:role/test-role",
			"metrics:",
			"- name: BucketSizeBytes",
		}

		for _, expected := range expectedStrings {
			if !strings.Contains(config, expected) {
				t.Errorf("expected config to contain '%s', got:\n%s", expected, config)
			}
		}
	})

	t.Run("should handle complex tagemon spec", func(t *testing.T) {
		tagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "complex-tagemon",
				Namespace: "default",
			},
			Spec: v1alpha1.TagemonSpec{
				Type:    "AWS/RDS",
				Regions: []v1alpha1.AWSRegion{"us-east-1", "us-west-2"},
				Roles: []v1alpha1.AWSRole{
					{
						RoleArn:    "arn:aws:iam::123456789012:role/role1",
						ExternalId: "ext123",
					},
					{
						RoleArn: "arn:aws:iam::987654321098:role/role2",
					},
				},
				SearchTags: []v1alpha1.TagemonTag{
					{Key: "Environment", Value: "production"},
					{Key: "Team", Value: "platform"},
				},
				Statistics: []v1alpha1.Statistics{"Average", "Sum"},
				Period:     int32Ptr(300),
				Length:     int32Ptr(3600),
				Metrics: []v1alpha1.TagemonMetric{
					{
						Name:       "CPUUtilization",
						Statistics: []v1alpha1.Statistics{"Maximum"},
						Period:     int32Ptr(60),
					},
					{Name: "DatabaseConnections"},
				},
				ExportedTagsOnMetrics: []string{"Name", "Environment"},
			},
		}

		config, err := reconciler.generateYACEConfig(tagemon)
		if err != nil {
			t.Fatalf("generateYACEConfig failed: %v", err)
		}

		expectedStrings := []string{
			"type: AWS/RDS",
			"us-west-2",
			"externalId: ext123",
			"key: Environment",
			"value: production",
			"- Maximum",  // Metric-specific statistics
			"period: 60", // Metric-specific period
		}

		for _, expected := range expectedStrings {
			if !strings.Contains(config, expected) {
				t.Errorf("expected config to contain '%s', got:\n%s", expected, config)
			}
		}
	})

	t.Run("should handle empty optional fields", func(t *testing.T) {
		tagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "minimal-tagemon",
				Namespace: "default",
			},
			Spec: v1alpha1.TagemonSpec{
				Type:    "AWS/Lambda",
				Regions: []v1alpha1.AWSRegion{"eu-west-1"},
				Roles: []v1alpha1.AWSRole{
					{RoleArn: "arn:aws:iam::111111111111:role/lambda-role"},
				},
				Metrics: []v1alpha1.TagemonMetric{
					{Name: "Duration"},
				},
				SearchTags:            []v1alpha1.TagemonTag{},
				ExportedTagsOnMetrics: []string{},
			},
		}

		config, err := reconciler.generateYACEConfig(tagemon)
		if err != nil {
			t.Fatalf("generateYACEConfig failed: %v", err)
		}

		if !strings.Contains(config, "AWS/Lambda") {
			t.Error("expected config to contain service type")
		}
		if !strings.Contains(config, "Duration") {
			t.Error("expected config to contain metric name")
		}
	})
}

func TestSetMetricOrGlobalValue(t *testing.T) {
	t.Run("should use metric-specific value when available", func(t *testing.T) {
		metric := &v1alpha1.TagemonMetric{
			Period: int32Ptr(60),
		}
		spec := &v1alpha1.TagemonSpec{
			Period: int32Ptr(300),
		}
		result := make(map[string]interface{})

		setMetricOrGlobalValue(result, "period", metric, spec)

		if result["period"] != int32(60) {
			t.Errorf("expected metric-specific period 60, got %v", result["period"])
		}
	})

	t.Run("should use global value when metric-specific not available", func(t *testing.T) {
		metric := &v1alpha1.TagemonMetric{}
		spec := &v1alpha1.TagemonSpec{
			Period: int32Ptr(300),
		}
		result := make(map[string]interface{})

		setMetricOrGlobalValue(result, "period", metric, spec)

		if result["period"] != int32(300) {
			t.Errorf("expected global period 300, got %v", result["period"])
		}
	})

	t.Run("should handle statistics arrays", func(t *testing.T) {
		metric := &v1alpha1.TagemonMetric{
			Statistics: []v1alpha1.Statistics{"Average", "Sum"},
		}
		spec := &v1alpha1.TagemonSpec{
			Statistics: []v1alpha1.Statistics{"Maximum"},
		}
		result := make(map[string]interface{})

		setMetricOrGlobalValue(result, "statistics", metric, spec)

		stats, ok := result["statistics"].([]v1alpha1.Statistics)
		if !ok {
			t.Fatalf("expected statistics to be []Statistics, got %T", result["statistics"])
		}
		if len(stats) != 2 {
			t.Errorf("expected 2 statistics, got %d", len(stats))
		}
		if stats[0] != "Average" || stats[1] != "Sum" {
			t.Errorf("expected [Average, Sum], got %v", stats)
		}
	})

	t.Run("should handle boolean values", func(t *testing.T) {
		tests := []struct {
			name            string
			metricNilToZero *bool
			globalNilToZero *bool
			expected        interface{}
		}{
			{
				name:            "metric true, global false",
				metricNilToZero: boolPtr(true),
				globalNilToZero: boolPtr(false),
				expected:        true,
			},
			{
				name:            "metric nil, global true",
				metricNilToZero: nil,
				globalNilToZero: boolPtr(true),
				expected:        true,
			},
			{
				name:            "both nil",
				metricNilToZero: nil,
				globalNilToZero: nil,
				expected:        nil,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				metric := &v1alpha1.TagemonMetric{
					NilToZero: tt.metricNilToZero,
				}
				spec := &v1alpha1.TagemonSpec{
					NilToZero: tt.globalNilToZero,
				}
				result := make(map[string]interface{})

				setMetricOrGlobalValue(result, "nilToZero", metric, spec)

				if tt.expected == nil {
					if _, exists := result["nilToZero"]; exists {
						t.Errorf("expected nilToZero not to be set, but got %v", result["nilToZero"])
					}
				} else {
					if result["nilToZero"] != tt.expected {
						t.Errorf("expected nilToZero to be %v, got %v", tt.expected, result["nilToZero"])
					}
				}
			})
		}
	})

	t.Run("should skip unknown keys", func(t *testing.T) {
		metric := &v1alpha1.TagemonMetric{}
		spec := &v1alpha1.TagemonSpec{}
		result := make(map[string]interface{})

		setMetricOrGlobalValue(result, "unknownField", metric, spec)

		if len(result) != 0 {
			t.Errorf("expected no values to be set for unknown field, got %v", result)
		}
	})
}

func TestGenerateShortHash(t *testing.T) {
	t.Run("should generate consistent hashes", func(t *testing.T) {
		input := "test-input"
		hash1 := generateShortHash(input)
		hash2 := generateShortHash(input)

		if hash1 != hash2 {
			t.Errorf("expected consistent hashes, got %s and %s", hash1, hash2)
		}
	})

	t.Run("should generate different hashes for different inputs", func(t *testing.T) {
		hash1 := generateShortHash("input1")
		hash2 := generateShortHash("input2")

		if hash1 == hash2 {
			t.Errorf("expected different hashes for different inputs, got same hash %s", hash1)
		}
	})

	t.Run("should generate fixed length hashes", func(t *testing.T) {
		inputs := []string{"short", "a very long input string", "123", ""}
		expectedLength := 5 // Based on implementation [:5]

		for _, input := range inputs {
			hash := generateShortHash(input)
			if len(hash) != expectedLength {
				t.Errorf("expected hash length %d for input %q, got %d", expectedLength, input, len(hash))
			}

			for _, char := range hash {
				if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
					t.Errorf("expected hexadecimal hash, got invalid character %c in %s", char, hash)
					break
				}
			}
		}
	})

	t.Run("should handle empty string", func(t *testing.T) {
		hash := generateShortHash("")
		if len(hash) != 5 {
			t.Errorf("expected hash length 5 for empty string, got %d", len(hash))
		}
	})
}

func TestBuildResourceName(t *testing.T) {
	reconciler := &Reconciler{
		TagsHandler: nil, // No tagshandler needed for this test
	}

	t.Run("should build resource name with service type and suffix", func(t *testing.T) {
		tagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-tagemon",
				Namespace: "default",
				UID:       "test-uid-123",
			},
			Spec: v1alpha1.TagemonSpec{
				Type: "AWS/S3",
			},
		}

		name := reconciler.buildResourceName(tagemon, "configmap")

		if !strings.Contains(name, "s3") {
			t.Errorf("expected name to contain service type, got %s", name)
		}

		if !strings.Contains(name, "configmap") {
			t.Errorf("expected name to contain suffix, got %s", name)
		}

		expectedMinLength := len("s3-configmap-") + 5 // 5-char hash
		if len(name) < expectedMinLength {
			t.Errorf("expected name length >= %d, got %d: %s", expectedMinLength, len(name), name)
		}
	})

	t.Run("should handle different suffixes", func(t *testing.T) {
		tagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
				UID:       "test-uid-456",
			},
			Spec: v1alpha1.TagemonSpec{
				Type: "AWS/RDS",
			},
		}

		configMapName := reconciler.buildResourceName(tagemon, "yace-cm")
		deploymentName := reconciler.buildResourceName(tagemon, "yace")

		if !strings.Contains(configMapName, "yace-cm") {
			t.Errorf("expected configmap name to contain yace-cm, got %s", configMapName)
		}
		if !strings.Contains(deploymentName, "yace") && !strings.Contains(deploymentName, "yace-cm") {
			t.Errorf("expected deployment name to contain yace, got %s", deploymentName)
		}

		if configMapName == deploymentName {
			t.Errorf("expected different resource names, both got %s", configMapName)
		}
	})

	t.Run("should generate consistent names regardless of input length", func(t *testing.T) {
		shortTagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "a",
				Namespace: "default",
				UID:       "short-uid",
			},
			Spec: v1alpha1.TagemonSpec{Type: "AWS/S3"},
		}

		longTagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "very-long-tagemon-name-that-exceeds-normal-lengths",
				Namespace: "default",
				UID:       "very-long-uid-that-is-much-longer-than-typical",
			},
			Spec: v1alpha1.TagemonSpec{Type: "AWS/ElasticLoadBalancing"},
		}

		shortName := reconciler.buildResourceName(shortTagemon, "test")
		longName := reconciler.buildResourceName(longTagemon, "test")

		shortHash := shortName[strings.LastIndex(shortName, "-")+1:]
		longHash := longName[strings.LastIndex(longName, "-")+1:]

		if len(shortHash) != len(longHash) {
			t.Errorf("expected consistent hash lengths, got %d and %d", len(shortHash), len(longHash))
		}
	})

	t.Run("should handle special characters in names", func(t *testing.T) {
		tagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test/name",
				Namespace: "default",
				UID:       "test-uid-789",
			},
			Spec: v1alpha1.TagemonSpec{
				Type: "AWS/ApplicationELB",
			},
		}

		name := reconciler.buildResourceName(tagemon, "resource")

		if !strings.Contains(name, "applicationelb") {
			t.Errorf("expected cleaned service type in name, got %s", name)
		}

		for _, char := range name {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				t.Errorf("expected DNS-valid name, found invalid character %c in %s", char, name)
				break
			}
		}
	})

	t.Run("should handle NamePrefix when specified", func(t *testing.T) {
		tagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-tagemon",
				Namespace: "default",
				UID:       "test-uid-999",
			},
			Spec: v1alpha1.TagemonSpec{
				Type:       "AWS/S3",
				NamePrefix: "custom-prefix",
			},
		}

		name := reconciler.buildResourceName(tagemon, "test")

		if !strings.HasPrefix(name, "custom-prefix-") {
			t.Errorf("expected name to start with custom-prefix-, got %s", name)
		}
	})
}

// =============================================================================
// LIFECYCLE HANDLER TESTS
// =============================================================================

func TestHandleCreate(t *testing.T) {
	t.Run("should orchestrate complete resource creation successfully", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)

		tagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-create",
				Namespace:  "default",
				Generation: 1,
			},
			Spec: v1alpha1.TagemonSpec{
				Type:    "AWS/S3",
				Regions: []v1alpha1.AWSRegion{"us-east-1"},
				Roles: []v1alpha1.AWSRole{
					{RoleArn: "arn:aws:iam::123456789012:role/test-role"},
				},
				Statistics: []v1alpha1.Statistics{"Sum"},
				Period:     int32Ptr(300),
				Length:     int32Ptr(3600),
				Metrics: []v1alpha1.TagemonMetric{
					{Name: "BucketSizeBytes"},
				},
			},
			Status: v1alpha1.TagemonStatus{
				ObservedGeneration: 0, // Triggers create
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(tagemon).
			WithStatusSubresource(&v1alpha1.Tagemon{}).
			Build()

		reconciler := &Reconciler{
			Client:      fakeClient,
			Scheme:      scheme,
			Config:      &confighandler.Config{ServiceAccountName: "test-sa"},
			TagsHandler: nil, // No tagshandler needed for this test
		}

		ctx := context.TODO()

		result, err := reconciler.handleCreate(ctx, tagemon)
		if err != nil {
			t.Fatalf("handleCreate failed: %v", err)
		}
		if result.RequeueAfter > 0 {
			t.Error("expected no requeue on successful create")
		}

		if tagemon.Status.DeploymentID == "" {
			t.Error("expected DeploymentID to be generated")
		}

		if !controllerutil.ContainsFinalizer(tagemon, finalizerName) {
			t.Error("expected finalizer to be added")
		}

		configMaps := &corev1.ConfigMapList{}
		err = fakeClient.List(ctx, configMaps, client.InNamespace(tagemon.Namespace))
		if err != nil {
			t.Fatalf("failed to list ConfigMaps: %v", err)
		}
		if len(configMaps.Items) != 1 {
			t.Errorf("expected 1 ConfigMap, got %d", len(configMaps.Items))
		}

		deployments := &appsv1.DeploymentList{}
		err = fakeClient.List(ctx, deployments, client.InNamespace(tagemon.Namespace))
		if err != nil {
			t.Fatalf("failed to list Deployments: %v", err)
		}
		if len(deployments.Items) != 1 {
			t.Errorf("expected 1 Deployment, got %d", len(deployments.Items))
		}

		if tagemon.Status.ConfigMapName == "" {
			t.Error("expected ConfigMapName to be set in status")
		}
		if tagemon.Status.DeploymentName == "" {
			t.Error("expected DeploymentName to be set in status")
		}
	})

	t.Run("should handle UUID generation and finalizer addition", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)

		tagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "uuid-test",
				Namespace:  "default",
				Generation: 1,
			},
			Spec: v1alpha1.TagemonSpec{
				Type:    "AWS/S3",
				Regions: []v1alpha1.AWSRegion{"us-east-1"},
				Roles: []v1alpha1.AWSRole{
					{RoleArn: "arn:aws:iam::123456789012:role/test-role"},
				},
				Statistics: []v1alpha1.Statistics{"Sum"},
				Period:     int32Ptr(300),
				Length:     int32Ptr(3600),
				Metrics: []v1alpha1.TagemonMetric{
					{Name: "BucketSizeBytes"},
				},
			},
			Status: v1alpha1.TagemonStatus{
				ObservedGeneration: 0, // Triggers create
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(tagemon).
			WithStatusSubresource(&v1alpha1.Tagemon{}).
			Build()

		reconciler := &Reconciler{
			Client:      fakeClient,
			Scheme:      scheme,
			Config:      &confighandler.Config{ServiceAccountName: "test-sa"},
			TagsHandler: nil, // No tagshandler needed for this test
		}

		ctx := context.TODO()

		result, err := reconciler.handleCreate(ctx, tagemon)
		if err != nil {
			t.Fatalf("handleCreate failed: %v", err)
		}
		if result.RequeueAfter > 0 {
			t.Error("expected no requeue on successful create")
		}

		if tagemon.Status.DeploymentID == "" {
			t.Error("expected DeploymentID to be generated")
		}
		if len(tagemon.Status.DeploymentID) != 36 { // Standard UUID length
			t.Errorf("expected UUID to be 36 characters, got %d", len(tagemon.Status.DeploymentID))
		}

		if !controllerutil.ContainsFinalizer(tagemon, finalizerName) {
			t.Error("expected finalizer to be added")
		}
	})

	t.Run("should handle ConfigMap creation failure", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)

		tagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "create-failure-test",
				Namespace: "default",
			},
			Spec: v1alpha1.TagemonSpec{
				Type:    "AWS/S3",
				Regions: []v1alpha1.AWSRegion{"us-east-1"},
				Roles: []v1alpha1.AWSRole{
					{RoleArn: "arn:aws:iam::123456789012:role/test-role"},
				},
				Statistics: []v1alpha1.Statistics{"Sum"},
				Period:     int32Ptr(300),
				Length:     int32Ptr(3600),
				Metrics: []v1alpha1.TagemonMetric{
					{Name: "BucketSizeBytes"},
				},
			},
		}

		reconciler := &Reconciler{
			Client:      fake.NewClientBuilder().WithScheme(scheme).Build(),
			Scheme:      scheme,
			Config:      &confighandler.Config{ServiceAccountName: "test-sa"},
			TagsHandler: nil, // No tagshandler needed for this test
		}

		configMapName := reconciler.buildResourceName(tagemon, "yace-cm")
		existingConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: tagemon.Namespace,
			},
			Data: map[string]string{
				"config.yml": "existing-config",
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(tagemon, existingConfigMap).
			WithStatusSubresource(&v1alpha1.Tagemon{}).
			Build()

		reconciler.Client = fakeClient

		ctx := context.TODO()

		_, err := reconciler.handleCreate(ctx, tagemon)
		if err == nil {
			t.Fatal("expected handleCreate to fail due to existing ConfigMap")
		}
		if !strings.Contains(err.Error(), "already exists") {
			t.Errorf("expected 'already exists' error, got: %v", err)
		}
	})
}

func TestHandleModify(t *testing.T) {
	t.Run("should orchestrate complete resource modification successfully", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)

		tagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-modify",
				Namespace: "default",
				UID:       "modify-uid-456",
			},
			Spec: v1alpha1.TagemonSpec{
				Type:    "AWS/RDS",
				Regions: []v1alpha1.AWSRegion{"us-east-1"},
				Roles: []v1alpha1.AWSRole{
					{RoleArn: "arn:aws:iam::123456789012:role/test-role"},
				},
				Statistics: []v1alpha1.Statistics{"Average"},
				Period:     int32Ptr(300),
				Length:     int32Ptr(3600),
				Metrics: []v1alpha1.TagemonMetric{
					{Name: "CPUUtilization"},
				},
			},
		}

		reconciler := &Reconciler{
			Client:      fake.NewClientBuilder().WithScheme(scheme).Build(),
			Scheme:      scheme,
			Config:      &confighandler.Config{ServiceAccountName: "test-sa"},
			TagsHandler: nil, // No tagshandler needed for this test
		}

		configMapName := reconciler.buildResourceName(tagemon, "yace-cm")
		existingConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: tagemon.Namespace,
			},
			Data: map[string]string{
				"config.yml": "old-config",
			},
		}

		deploymentName := reconciler.buildResourceName(tagemon, "yace")
		existingDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      deploymentName,
				Namespace: tagemon.Namespace,
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "yace",
								Image: "prometheuscommunity/yet-another-cloudwatch-exporter:v0.62.1",
							},
						},
					},
				},
			},
		}

		tagemon.Status.ConfigMapName = configMapName
		tagemon.Status.DeploymentName = deploymentName

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(tagemon, existingConfigMap, existingDeployment).
			WithStatusSubresource(&v1alpha1.Tagemon{}).
			Build()

		reconciler.Client = fakeClient

		ctx := context.TODO()

		result, err := reconciler.handleModify(ctx, tagemon)
		if err != nil {
			t.Fatalf("handleModify failed: %v", err)
		}
		if result.RequeueAfter > 0 {
			t.Error("expected no requeue on successful modify")
		}

		updatedConfigMap := &corev1.ConfigMap{}
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      configMapName,
			Namespace: tagemon.Namespace,
		}, updatedConfigMap)
		if err != nil {
			t.Fatalf("failed to get updated ConfigMap: %v", err)
		}

		if updatedConfigMap.Data["config.yml"] == "old-config" {
			t.Error("expected ConfigMap to be updated with new config")
		}

		updatedDeployment := &appsv1.Deployment{}
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      tagemon.Status.DeploymentName,
			Namespace: tagemon.Namespace,
		}, updatedDeployment)
		if err != nil {
			t.Fatalf("failed to get updated Deployment: %v", err)
		}

		annotations := updatedDeployment.Spec.Template.Annotations
		if annotations == nil {
			t.Error("expected Deployment to have annotations")
		} else if _, exists := annotations["tagemon.io/restartedAt"]; !exists {
			t.Error("expected Deployment to have tagemon.io/restartedAt annotation")
		}
	})

	t.Run("should skip modification when no changes detected", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)

		tagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "no-change-test",
				Namespace: "default",
				UID:       "nochange-uid-789",
			},
			Spec: v1alpha1.TagemonSpec{
				Type:    "AWS/RDS",
				Regions: []v1alpha1.AWSRegion{"us-east-1"},
				Roles: []v1alpha1.AWSRole{
					{RoleArn: "arn:aws:iam::123456789012:role/test-role"},
				},
				Statistics: []v1alpha1.Statistics{"Average"},
				Period:     int32Ptr(300),
				Length:     int32Ptr(3600),
				Metrics: []v1alpha1.TagemonMetric{
					{Name: "CPUUtilization"},
				},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(tagemon).
			WithStatusSubresource(&v1alpha1.Tagemon{}).
			Build()

		reconciler := &Reconciler{
			Client:      fakeClient,
			Scheme:      scheme,
			Config:      &confighandler.Config{ServiceAccountName: "test-sa"},
			TagsHandler: nil, // No tagshandler needed for this test
		}

		currentConfig, err := reconciler.generateYACEConfig(tagemon)
		if err != nil {
			t.Fatalf("failed to generate YACE config: %v", err)
		}

		configMapName := reconciler.buildResourceName(tagemon, "yace-cm")
		existingConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: tagemon.Namespace,
			},
			Data: map[string]string{
				"config.yml": currentConfig,
			},
		}

		tagemon.Status.ConfigMapName = configMapName

		err = fakeClient.Create(context.TODO(), existingConfigMap)
		if err != nil {
			t.Fatalf("failed to create existing ConfigMap: %v", err)
		}

		ctx := context.TODO()

		result, err := reconciler.handleModify(ctx, tagemon)
		if err != nil {
			t.Fatalf("handleModify failed: %v", err)
		}
		if result.RequeueAfter > 0 {
			t.Error("expected no requeue when no changes detected")
		}

		unchangedConfigMap := &corev1.ConfigMap{}
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      configMapName,
			Namespace: tagemon.Namespace,
		}, unchangedConfigMap)
		if err != nil {
			t.Fatalf("failed to get ConfigMap: %v", err)
		}

		if unchangedConfigMap.Data["config.yml"] != currentConfig {
			t.Error("expected ConfigMap to remain unchanged")
		}
	})

	t.Run("should handle ConfigMap update failure", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)

		tagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "update-failure-test",
				Namespace: "default",
			},
			Spec: v1alpha1.TagemonSpec{
				Type:    "AWS/S3",
				Regions: []v1alpha1.AWSRegion{"us-east-1"},
				Roles: []v1alpha1.AWSRole{
					{RoleArn: "arn:aws:iam::123456789012:role/test-role"},
				},
				Statistics: []v1alpha1.Statistics{"Sum"},
				Period:     int32Ptr(300),
				Length:     int32Ptr(3600),
				Metrics: []v1alpha1.TagemonMetric{
					{Name: "BucketSizeBytes"},
				},
			},
			Status: v1alpha1.TagemonStatus{
				ConfigMapName: "non-existent-configmap",
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(tagemon).
			WithStatusSubresource(&v1alpha1.Tagemon{}).
			Build()

		reconciler := &Reconciler{
			Client:      fakeClient,
			Scheme:      scheme,
			Config:      &confighandler.Config{ServiceAccountName: "test-sa"},
			TagsHandler: nil, // No tagshandler needed for this test
		}

		ctx := context.TODO()

		result, err := reconciler.handleModify(ctx, tagemon)
		if err != nil {
			t.Fatalf("handleModify should handle missing ConfigMap gracefully, got error: %v", err)
		}
		if result.RequeueAfter > 0 {
			t.Error("expected no requeue for missing ConfigMap")
		}
	})
}

func TestHandleDelete(t *testing.T) {
	t.Run("should orchestrate complete resource deletion successfully", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)

		tagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-delete",
				Namespace:  "default",
				UID:        "delete-uid-123",
				Finalizers: []string{finalizerName},
			},
			Spec: v1alpha1.TagemonSpec{
				Type:    "AWS/S3",
				Regions: []v1alpha1.AWSRegion{"us-east-1"},
				Roles: []v1alpha1.AWSRole{
					{RoleArn: "arn:aws:iam::123456789012:role/test-role"},
				},
			},
		}

		reconciler := &Reconciler{
			Client:      fake.NewClientBuilder().WithScheme(scheme).Build(),
			Scheme:      scheme,
			Config:      &confighandler.Config{ServiceAccountName: "test-sa"},
			TagsHandler: nil, // No tagshandler needed for this test
		}

		configMapName := reconciler.buildResourceName(tagemon, "yace-cm")
		deploymentName := reconciler.buildResourceName(tagemon, "yace")

		existingConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: tagemon.Namespace,
			},
			Data: map[string]string{
				"config.yml": "test-config",
			},
		}

		existingDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      deploymentName,
				Namespace: tagemon.Namespace,
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "yace",
								Image: "test-image",
							},
						},
					},
				},
			},
		}

		tagemon.Status.ConfigMapName = configMapName
		tagemon.Status.DeploymentName = deploymentName

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(tagemon, existingConfigMap, existingDeployment).
			WithStatusSubresource(&v1alpha1.Tagemon{}).
			Build()

		reconciler.Client = fakeClient

		ctx := context.TODO()

		result, err := reconciler.handleDelete(ctx, tagemon)
		if err != nil {
			t.Fatalf("handleDelete failed: %v", err)
		}
		if result.RequeueAfter > 0 {
			t.Error("expected no requeue on successful delete")
		}

		deletedConfigMap := &corev1.ConfigMap{}
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      configMapName,
			Namespace: tagemon.Namespace,
		}, deletedConfigMap)
		if err == nil {
			t.Error("expected ConfigMap to be deleted")
		}

		deletedDeployment := &appsv1.Deployment{}
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      deploymentName,
			Namespace: tagemon.Namespace,
		}, deletedDeployment)
		if err == nil {
			t.Error("expected Deployment to be deleted")
		}

		if controllerutil.ContainsFinalizer(tagemon, finalizerName) {
			t.Error("expected finalizer to be removed from the original Tagemon object")
		}
	})

	t.Run("should handle missing resources gracefully", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)

		tagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "missing-resources-test",
				Namespace:  "default",
				Finalizers: []string{finalizerName},
			},
			Spec: v1alpha1.TagemonSpec{
				Type: "AWS/S3",
			},
			Status: v1alpha1.TagemonStatus{
				ConfigMapName:  "non-existent-configmap",
				DeploymentName: "non-existent-deployment",
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(tagemon).
			WithStatusSubresource(&v1alpha1.Tagemon{}).
			Build()

		reconciler := &Reconciler{
			Client:      fakeClient,
			Scheme:      scheme,
			Config:      &confighandler.Config{ServiceAccountName: "test-sa"},
			TagsHandler: nil, // No tagshandler needed for this test
		}

		ctx := context.TODO()

		result, err := reconciler.handleDelete(ctx, tagemon)
		if err != nil {
			t.Fatalf("handleDelete should handle missing resources gracefully, got error: %v", err)
		}
		if result.RequeueAfter > 0 {
			t.Error("expected no requeue when handling missing resources")
		}

		if controllerutil.ContainsFinalizer(tagemon, finalizerName) {
			t.Error("expected finalizer to be removed even when resources are missing")
		}
	})

	t.Run("should handle empty status fields gracefully", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)

		tagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "empty-status-test",
				Namespace:  "default",
				Finalizers: []string{finalizerName},
			},
			Spec: v1alpha1.TagemonSpec{
				Type: "AWS/S3",
			},
			Status: v1alpha1.TagemonStatus{},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(tagemon).
			WithStatusSubresource(&v1alpha1.Tagemon{}).
			Build()

		reconciler := &Reconciler{
			Client:      fakeClient,
			Scheme:      scheme,
			Config:      &confighandler.Config{ServiceAccountName: "test-sa"},
			TagsHandler: nil, // No tagshandler needed for this test
		}

		ctx := context.TODO()

		result, err := reconciler.handleDelete(ctx, tagemon)
		if err != nil {
			t.Fatalf("handleDelete should handle empty status gracefully, got error: %v", err)
		}
		if result.RequeueAfter > 0 {
			t.Error("expected no requeue with empty status")
		}

		if controllerutil.ContainsFinalizer(tagemon, finalizerName) {
			t.Error("expected finalizer to be removed even with empty status")
		}
	})
}

// =============================================================================
// RESOURCE OPERATION TESTS
// =============================================================================

func TestCreateConfigMap(t *testing.T) {
	t.Run("should create ConfigMap with correct YACE configuration", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)

		tagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-configmap",
				Namespace: "default",
				UID:       "test-uid-123",
			},
			Spec: v1alpha1.TagemonSpec{
				Type:    "AWS/S3",
				Regions: []v1alpha1.AWSRegion{"us-east-1"},
				Roles: []v1alpha1.AWSRole{
					{RoleArn: "arn:aws:iam::123456789012:role/test-role"},
				},
				Statistics: []v1alpha1.Statistics{"Sum"},
				Period:     int32Ptr(300),
				Length:     int32Ptr(3600),
				Metrics: []v1alpha1.TagemonMetric{
					{Name: "BucketSizeBytes"},
				},
			},
			Status: v1alpha1.TagemonStatus{
				DeploymentID: "test-deployment-id",
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		reconciler := &Reconciler{
			Client:      fakeClient,
			Scheme:      scheme,
			Config:      &confighandler.Config{ServiceAccountName: "test-sa"},
			TagsHandler: nil, // No tagshandler needed for this test
		}

		yaceConfig, err := reconciler.generateYACEConfig(tagemon)
		if err != nil {
			t.Fatalf("failed to generate YACE config: %v", err)
		}

		ctx := context.TODO()
		err = reconciler.createConfigMap(ctx, tagemon, yaceConfig)
		if err != nil {
			t.Fatalf("createConfigMap failed: %v", err)
		}

		configMaps := &corev1.ConfigMapList{}
		err = fakeClient.List(ctx, configMaps, client.InNamespace(tagemon.Namespace))
		if err != nil {
			t.Fatalf("failed to list ConfigMaps: %v", err)
		}

		if len(configMaps.Items) != 1 {
			t.Errorf("expected 1 ConfigMap, got %d", len(configMaps.Items))
		}

		cm := configMaps.Items[0]

		if !strings.Contains(cm.Name, "s3") || !strings.Contains(cm.Name, "yace-cm") {
			t.Errorf("expected ConfigMap name to contain service type and suffix, got %s", cm.Name)
		}

		if config, exists := cm.Data["config.yml"]; !exists {
			t.Error("expected ConfigMap to have config.yml key")
		} else if config != yaceConfig {
			t.Error("expected ConfigMap data to match generated YACE config")
		}

		if deploymentID, exists := cm.Annotations["tagemon.io/deployment-id"]; !exists {
			t.Error("expected ConfigMap to have deployment-id annotation")
		} else if deploymentID != tagemon.Status.DeploymentID {
			t.Errorf("expected deployment-id annotation to be %s, got %s", tagemon.Status.DeploymentID, deploymentID)
		}

		if tagemon.Status.ConfigMapName != cm.Name {
			t.Errorf("expected status ConfigMapName to be %s, got %s", cm.Name, tagemon.Status.ConfigMapName)
		}
	})

	t.Run("should handle complex Tagemon spec", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)

		tagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "complex-tagemon",
				Namespace: "test-ns",
				UID:       "complex-uid-456",
			},
			Spec: v1alpha1.TagemonSpec{
				Type:       "AWS/RDS",
				NamePrefix: "prod",
				Regions:    []v1alpha1.AWSRegion{"us-east-1", "us-west-2"},
				Roles: []v1alpha1.AWSRole{
					{
						RoleArn:    "arn:aws:iam::123456789012:role/role1",
						ExternalId: "external123",
					},
				},
				SearchTags: []v1alpha1.TagemonTag{
					{Key: "Environment", Value: "production"},
				},
				Metrics: []v1alpha1.TagemonMetric{
					{
						Name:       "CPUUtilization",
						Statistics: []v1alpha1.Statistics{"Average", "Maximum"},
						Period:     int32Ptr(60),
					},
				},
			},
			Status: v1alpha1.TagemonStatus{
				DeploymentID: "complex-deployment-id",
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		reconciler := &Reconciler{
			Client:      fakeClient,
			Scheme:      scheme,
			Config:      &confighandler.Config{ServiceAccountName: "test-sa"},
			TagsHandler: nil, // No tagshandler needed for this test
		}

		yaceConfig, err := reconciler.generateYACEConfig(tagemon)
		if err != nil {
			t.Fatalf("failed to generate YACE config: %v", err)
		}

		ctx := context.TODO()
		err = reconciler.createConfigMap(ctx, tagemon, yaceConfig)
		if err != nil {
			t.Fatalf("createConfigMap failed: %v", err)
		}

		configMaps := &corev1.ConfigMapList{}
		err = fakeClient.List(ctx, configMaps, client.InNamespace(tagemon.Namespace))
		if err != nil {
			t.Fatalf("failed to list ConfigMaps: %v", err)
		}

		if len(configMaps.Items) != 1 {
			t.Errorf("expected 1 ConfigMap, got %d", len(configMaps.Items))
		}

		cm := configMaps.Items[0]
		if !strings.HasPrefix(cm.Name, "prod-") {
			t.Errorf("expected ConfigMap name to start with NamePrefix, got %s", cm.Name)
		}

		config := cm.Data["config.yml"]
		expectedElements := []string{
			"AWS/RDS",
			"us-west-2",
			"externalId: external123",
			"key: Environment",
			"value: production",
			"- Average",
			"- Maximum",
			"period: 60",
		}

		for _, element := range expectedElements {
			if !strings.Contains(config, element) {
				t.Errorf("expected config to contain '%s', got:\n%s", element, config)
			}
		}
	})
}

func TestCreateDeployment(t *testing.T) {
	t.Run("should create Deployment with correct configuration", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)

		tagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deployment",
				Namespace: "default",
				UID:       "test-uid-789",
			},
			Spec: v1alpha1.TagemonSpec{
				Type: "AWS/S3",
			},
			Status: v1alpha1.TagemonStatus{
				DeploymentID:  "test-deployment-id",
				ConfigMapName: "test-configmap",
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		reconciler := &Reconciler{
			Client:      fakeClient,
			Scheme:      scheme,
			Config:      &confighandler.Config{ServiceAccountName: "test-sa"},
			TagsHandler: nil, // No tagshandler needed for this test
		}

		ctx := context.TODO()
		err := reconciler.createDeployment(ctx, tagemon)
		if err != nil {
			t.Fatalf("createDeployment failed: %v", err)
		}

		deployments := &appsv1.DeploymentList{}
		err = fakeClient.List(ctx, deployments, client.InNamespace(tagemon.Namespace))
		if err != nil {
			t.Fatalf("failed to list Deployments: %v", err)
		}

		if len(deployments.Items) != 1 {
			t.Errorf("expected 1 Deployment, got %d", len(deployments.Items))
		}

		dep := deployments.Items[0]

		if !strings.Contains(dep.Name, "s3") || !strings.Contains(dep.Name, "yace") {
			t.Errorf("expected Deployment name to contain service type and suffix, got %s", dep.Name)
		}

		if deploymentID, exists := dep.Annotations["tagemon.io/deployment-id"]; !exists {
			t.Error("expected Deployment to have deployment-id annotation")
		} else if deploymentID != tagemon.Status.DeploymentID {
			t.Errorf("expected deployment-id annotation to be %s, got %s", tagemon.Status.DeploymentID, deploymentID)
		}

		containers := dep.Spec.Template.Spec.Containers
		if len(containers) != 1 {
			t.Errorf("expected 1 container, got %d", len(containers))
		}

		container := containers[0]
		if container.Name != "yace" {
			t.Errorf("expected container name to be 'yace', got %s", container.Name)
		}

		expectedImage := "prometheuscommunity/yet-another-cloudwatch-exporter:v0.62.1"
		if container.Image != expectedImage {
			t.Errorf("expected container image to be %s, got %s", expectedImage, container.Image)
		}

		if dep.Spec.Template.Spec.ServiceAccountName != reconciler.Config.ServiceAccountName {
			t.Errorf("expected ServiceAccountName to be %s, got %s", reconciler.Config.ServiceAccountName, dep.Spec.Template.Spec.ServiceAccountName)
		}

		if tagemon.Status.DeploymentName != dep.Name {
			t.Errorf("expected status DeploymentName to be %s, got %s", dep.Name, tagemon.Status.DeploymentName)
		}
	})

	t.Run("should apply pod resources when specified", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)

		tagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "resource-deployment",
				Namespace: "default",
				UID:       "resource-uid-999",
			},
			Spec: v1alpha1.TagemonSpec{
				Type: "AWS/RDS",
				PodResources: &v1alpha1.PodResources{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
			},
			Status: v1alpha1.TagemonStatus{
				DeploymentID:  "resource-deployment-id",
				ConfigMapName: "resource-configmap",
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		reconciler := &Reconciler{
			Client:      fakeClient,
			Scheme:      scheme,
			Config:      &confighandler.Config{ServiceAccountName: "test-sa"},
			TagsHandler: nil, // No tagshandler needed for this test
		}

		ctx := context.TODO()
		err := reconciler.createDeployment(ctx, tagemon)
		if err != nil {
			t.Fatalf("createDeployment failed: %v", err)
		}

		deployments := &appsv1.DeploymentList{}
		err = fakeClient.List(ctx, deployments, client.InNamespace(tagemon.Namespace))
		if err != nil {
			t.Fatalf("failed to list Deployments: %v", err)
		}

		dep := deployments.Items[0]
		container := dep.Spec.Template.Spec.Containers[0]

		if _, exists := container.Resources.Requests[corev1.ResourceCPU]; !exists {
			t.Error("expected CPU request to be set")
		}
		if _, exists := container.Resources.Requests[corev1.ResourceMemory]; !exists {
			t.Error("expected Memory request to be set")
		}

		if _, exists := container.Resources.Limits[corev1.ResourceCPU]; !exists {
			t.Error("expected CPU limit to be set")
		}
		if _, exists := container.Resources.Limits[corev1.ResourceMemory]; !exists {
			t.Error("expected Memory limit to be set")
		}
	})
}

func TestUpdateConfigMap(t *testing.T) {
	t.Run("should update ConfigMap and return true when content changes", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)

		tagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "update-test",
				Namespace: "default",
				UID:       "update-uid-123",
			},
			Spec: v1alpha1.TagemonSpec{
				Type:    "AWS/S3",
				Regions: []v1alpha1.AWSRegion{"us-east-1"},
				Roles: []v1alpha1.AWSRole{
					{RoleArn: "arn:aws:iam::123456789012:role/test-role"},
				},
				Statistics: []v1alpha1.Statistics{"Sum"},
				Period:     int32Ptr(300),
				Metrics: []v1alpha1.TagemonMetric{
					{Name: "BucketSizeBytes"},
				},
			},
		}

		reconciler := &Reconciler{
			Client:      fake.NewClientBuilder().WithScheme(scheme).Build(),
			Scheme:      scheme,
			Config:      &confighandler.Config{ServiceAccountName: "test-sa"},
			TagsHandler: nil, // No tagshandler needed for this test
		}

		configMapName := reconciler.buildResourceName(tagemon, "yace-cm")
		existingConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: tagemon.Namespace,
			},
			Data: map[string]string{
				"config.yml": "old-config-content",
			},
		}

		tagemon.Status.ConfigMapName = configMapName

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(existingConfigMap).
			Build()

		reconciler.Client = fakeClient

		ctx := context.TODO()

		changed, err := reconciler.updateConfigMap(ctx, tagemon)
		if err != nil {
			t.Fatalf("updateConfigMap failed: %v", err)
		}

		if !changed {
			t.Error("expected updateConfigMap to return true when content changes")
		}

		updatedConfigMap := &corev1.ConfigMap{}
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      configMapName,
			Namespace: tagemon.Namespace,
		}, updatedConfigMap)
		if err != nil {
			t.Fatalf("failed to get updated ConfigMap: %v", err)
		}

		if updatedConfigMap.Data["config.yml"] == "old-config-content" {
			t.Error("expected ConfigMap content to be updated")
		}

		newConfig := updatedConfigMap.Data["config.yml"]
		if !strings.Contains(newConfig, "AWS/S3") {
			t.Error("expected updated config to contain service type")
		}
	})

	t.Run("should return false when no content changes", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)

		tagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "no-change-test",
				Namespace: "default",
				UID:       "nochange-uid-456",
			},
			Spec: v1alpha1.TagemonSpec{
				Type:    "AWS/RDS",
				Regions: []v1alpha1.AWSRegion{"us-east-1"},
				Roles: []v1alpha1.AWSRole{
					{RoleArn: "arn:aws:iam::123456789012:role/test-role"},
				},
				Statistics: []v1alpha1.Statistics{"Average"},
				Period:     int32Ptr(300),
				Metrics: []v1alpha1.TagemonMetric{
					{Name: "CPUUtilization"},
				},
			},
		}

		reconciler := &Reconciler{
			Client:      fake.NewClientBuilder().WithScheme(scheme).Build(),
			Scheme:      scheme,
			Config:      &confighandler.Config{ServiceAccountName: "test-sa"},
			TagsHandler: nil, // No tagshandler needed for this test
		}

		currentConfig, err := reconciler.generateYACEConfig(tagemon)
		if err != nil {
			t.Fatalf("failed to generate YACE config: %v", err)
		}

		configMapName := reconciler.buildResourceName(tagemon, "yace-cm")
		existingConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: tagemon.Namespace,
			},
			Data: map[string]string{
				"config.yml": currentConfig,
			},
		}

		tagemon.Status.ConfigMapName = configMapName

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(existingConfigMap).
			Build()

		reconciler.Client = fakeClient

		ctx := context.TODO()

		changed, err := reconciler.updateConfigMap(ctx, tagemon)
		if err != nil {
			t.Fatalf("updateConfigMap failed: %v", err)
		}

		if changed {
			t.Error("expected updateConfigMap to return false when no content changes")
		}

		unchangedConfigMap := &corev1.ConfigMap{}
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      configMapName,
			Namespace: tagemon.Namespace,
		}, unchangedConfigMap)
		if err != nil {
			t.Fatalf("failed to get ConfigMap: %v", err)
		}

		if unchangedConfigMap.Data["config.yml"] != currentConfig {
			t.Error("expected ConfigMap to remain unchanged")
		}
	})
}

func TestDeleteConfigMap(t *testing.T) {
	t.Run("should delete existing ConfigMap", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)

		tagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "delete-cm-test",
				Namespace: "default",
			},
			Status: v1alpha1.TagemonStatus{
				ConfigMapName: "test-configmap",
			},
		}

		existingConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-configmap",
				Namespace: tagemon.Namespace,
			},
			Data: map[string]string{
				"config.yml": "test-config",
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(existingConfigMap).
			Build()

		reconciler := &Reconciler{
			Client:      fakeClient,
			Scheme:      scheme,
			Config:      &confighandler.Config{ServiceAccountName: "test-sa"},
			TagsHandler: nil, // No tagshandler needed for this test
		}

		ctx := context.TODO()

		err := reconciler.deleteConfigMap(ctx, tagemon)
		if err != nil {
			t.Fatalf("deleteConfigMap failed: %v", err)
		}

		deletedConfigMap := &corev1.ConfigMap{}
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      "test-configmap",
			Namespace: tagemon.Namespace,
		}, deletedConfigMap)
		if err == nil {
			t.Error("expected ConfigMap to be deleted")
		}
	})

	t.Run("should handle missing ConfigMap gracefully", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)

		tagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "missing-cm-test",
				Namespace: "default",
			},
			Status: v1alpha1.TagemonStatus{
				ConfigMapName: "non-existent-configmap",
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		reconciler := &Reconciler{
			Client:      fakeClient,
			Scheme:      scheme,
			Config:      &confighandler.Config{ServiceAccountName: "test-sa"},
			TagsHandler: nil, // No tagshandler needed for this test
		}

		ctx := context.TODO()

		err := reconciler.deleteConfigMap(ctx, tagemon)
		if err != nil {
			t.Fatalf("deleteConfigMap should handle missing ConfigMap gracefully, got error: %v", err)
		}
	})
}

func TestDeleteDeployment(t *testing.T) {
	t.Run("should delete existing Deployment", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)

		tagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "delete-dep-test",
				Namespace: "default",
			},
			Status: v1alpha1.TagemonStatus{
				DeploymentName: "test-deployment",
			},
		}

		existingDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deployment",
				Namespace: tagemon.Namespace,
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "yace",
								Image: "test-image",
							},
						},
					},
				},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(existingDeployment).
			Build()

		reconciler := &Reconciler{
			Client:      fakeClient,
			Scheme:      scheme,
			Config:      &confighandler.Config{ServiceAccountName: "test-sa"},
			TagsHandler: nil, // No tagshandler needed for this test
		}

		ctx := context.TODO()

		err := reconciler.deleteDeployment(ctx, tagemon)
		if err != nil {
			t.Fatalf("deleteDeployment failed: %v", err)
		}

		deletedDeployment := &appsv1.Deployment{}
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      "test-deployment",
			Namespace: tagemon.Namespace,
		}, deletedDeployment)
		if err == nil {
			t.Error("expected Deployment to be deleted")
		}
	})

	t.Run("should handle missing Deployment gracefully", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)

		tagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "missing-dep-test",
				Namespace: "default",
			},
			Status: v1alpha1.TagemonStatus{
				DeploymentName: "non-existent-deployment",
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		reconciler := &Reconciler{
			Client:      fakeClient,
			Scheme:      scheme,
			Config:      &confighandler.Config{ServiceAccountName: "test-sa"},
			TagsHandler: nil, // No tagshandler needed for this test
		}

		ctx := context.TODO()

		err := reconciler.deleteDeployment(ctx, tagemon)
		if err != nil {
			t.Fatalf("deleteDeployment should handle missing Deployment gracefully, got error: %v", err)
		}
	})
}

func TestFinalizerHandling(t *testing.T) {
	t.Run("should add finalizer correctly", func(t *testing.T) {
		tagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "finalizer-test",
				Namespace: "default",
			},
		}

		if controllerutil.ContainsFinalizer(tagemon, finalizerName) {
			t.Error("expected no finalizer initially")
		}

		controllerutil.AddFinalizer(tagemon, finalizerName)

		if !controllerutil.ContainsFinalizer(tagemon, finalizerName) {
			t.Error("expected finalizer to be added")
		}

		found := false
		for _, finalizer := range tagemon.GetFinalizers() {
			if finalizer == finalizerName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected finalizer %s to be in finalizers list %v", finalizerName, tagemon.GetFinalizers())
		}
	})

	t.Run("should remove finalizer correctly", func(t *testing.T) {
		tagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "finalizer-remove-test",
				Namespace:  "default",
				Finalizers: []string{finalizerName, "other.finalizer"},
			},
		}

		if !controllerutil.ContainsFinalizer(tagemon, finalizerName) {
			t.Error("expected finalizer to be present initially")
		}

		controllerutil.RemoveFinalizer(tagemon, finalizerName)

		if controllerutil.ContainsFinalizer(tagemon, finalizerName) {
			t.Error("expected finalizer to be removed")
		}

		if !controllerutil.ContainsFinalizer(tagemon, "other.finalizer") {
			t.Error("expected other finalizer to remain")
		}

		for _, finalizer := range tagemon.GetFinalizers() {
			if finalizer == finalizerName {
				t.Errorf("expected finalizer %s to be removed from finalizers list %v", finalizerName, tagemon.GetFinalizers())
			}
		}
	})
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

func int32Ptr(i int32) *int32 {
	return &i
}

func boolPtr(b bool) *bool {
	return &b
}
