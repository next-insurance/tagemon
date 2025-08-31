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

package thresholdshandler

import (
	"context"
	"reflect"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/next-insurance/tagemon-dev/api/v1alpha1"
)

// =============================================================================
// UTILITY FUNCTION TESTS
// =============================================================================

func TestThresholdMonitor_Initialization(t *testing.T) {
	t.Run("should initialize monitor correctly with valid kube client", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		monitor := &ThresholdMonitor{
			client: fakeClient,
		}

		if monitor.client == nil {
			t.Error("expected client to be set")
		}
	})
}

func TestExtractServiceFromTagemonType(t *testing.T) {
	monitor := &ThresholdMonitor{}

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standard EC2 type",
			input:    "AWS/EC2",
			expected: "ec2",
		},
		{
			name:     "standard RDS type",
			input:    "AWS/RDS",
			expected: "rds",
		},
		{
			name:     "standard Lambda type",
			input:    "AWS/Lambda",
			expected: "lambda",
		},
		{
			name:     "ApplicationELB special case",
			input:    "AWS/ApplicationELB",
			expected: "elasticloadbalancing",
		},
		{
			name:     "NetworkELB special case",
			input:    "AWS/NetworkELB",
			expected: "elasticloadbalancing",
		},
		{
			name:     "ELB special case",
			input:    "AWS/ELB",
			expected: "elasticloadbalancing",
		},
		{
			name:     "mixed case service name",
			input:    "AWS/S3",
			expected: "s3",
		},
		{
			name:     "non-AWS type",
			input:    "CustomType",
			expected: "customtype",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "case sensitivity test",
			input:    "aws/ec2",
			expected: "aws/ec2",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := monitor.extractServiceFromTagemonType(tc.input)
			if result != tc.expected {
				t.Errorf("expected extractServiceFromTagemonType('%s') to return '%s', got '%s'", tc.input, tc.expected, result)
			}
		})
	}
}

func TestGroupThresholdTagsByType(t *testing.T) {
	monitor := &ThresholdMonitor{}

	t.Run("should group tags by type correctly", func(t *testing.T) {
		tagemons := []v1alpha1.Tagemon{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tagemon1",
					Namespace: "default",
				},
				Spec: v1alpha1.TagemonSpec{
					Type:          "AWS/EC2",
					ThresholdTags: []string{"MaxCPU", "MinMemory"},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tagemon2",
					Namespace: "default",
				},
				Spec: v1alpha1.TagemonSpec{
					Type:          "AWS/EC2",
					ThresholdTags: []string{"MaxCPU", "NetworkIn"},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tagemon3",
					Namespace: "default",
				},
				Spec: v1alpha1.TagemonSpec{
					Type:          "AWS/RDS",
					ThresholdTags: []string{"DBConnections"},
				},
			},
		}

		result := monitor.groupThresholdTagsByType(tagemons)

		if len(result) != 2 {
			t.Errorf("expected 2 grouped types, got %d", len(result))
		}

		var ec2Group, rdsGroup *ThresholdTagsByType
		for i := range result {
			switch result[i].TagemonType {
			case "AWS/EC2":
				ec2Group = &result[i]
			case "AWS/RDS":
				rdsGroup = &result[i]
			}
		}

		if ec2Group == nil {
			t.Error("expected to find AWS/EC2 group")
		} else {
			if len(ec2Group.Tags) != 3 {
				t.Errorf("expected EC2 group to have 3 tags, got %d: %v", len(ec2Group.Tags), ec2Group.Tags)
			}
			expectedTags := map[string]bool{"MaxCPU": true, "MinMemory": true, "NetworkIn": true}
			for _, tag := range ec2Group.Tags {
				if !expectedTags[tag] {
					t.Errorf("unexpected tag in EC2 group: %s", tag)
				}
			}
		}

		if rdsGroup == nil {
			t.Error("expected to find AWS/RDS group")
		} else {
			if len(rdsGroup.Tags) != 1 {
				t.Errorf("expected RDS group to have 1 tag, got %d: %v", len(rdsGroup.Tags), rdsGroup.Tags)
			}
			if rdsGroup.Tags[0] != "DBConnections" {
				t.Errorf("expected RDS tag to be 'DBConnections', got '%s'", rdsGroup.Tags[0])
			}
		}
	})

	t.Run("should handle empty threshold tags", func(t *testing.T) {
		tagemons := []v1alpha1.Tagemon{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-tags",
					Namespace: "default",
				},
				Spec: v1alpha1.TagemonSpec{
					Type:          "AWS/S3",
					ThresholdTags: []string{},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nil-tags",
					Namespace: "default",
				},
				Spec: v1alpha1.TagemonSpec{
					Type:          "AWS/Lambda",
					ThresholdTags: nil,
				},
			},
		}

		result := monitor.groupThresholdTagsByType(tagemons)

		if len(result) != 0 {
			t.Errorf("expected 0 grouped types for empty threshold tags, got %d", len(result))
		}
	})

	t.Run("should handle empty tag values", func(t *testing.T) {
		tagemons := []v1alpha1.Tagemon{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mixed-tags",
					Namespace: "default",
				},
				Spec: v1alpha1.TagemonSpec{
					Type:          "AWS/EC2",
					ThresholdTags: []string{"ValidTag", "", "AnotherTag", ""},
				},
			},
		}

		result := monitor.groupThresholdTagsByType(tagemons)

		if len(result) != 1 {
			t.Errorf("expected 1 grouped type, got %d", len(result))
		}

		if len(result[0].Tags) != 2 {
			t.Errorf("expected 2 valid tags, got %d: %v", len(result[0].Tags), result[0].Tags)
		}

		expectedTags := map[string]bool{"ValidTag": true, "AnotherTag": true}
		for _, tag := range result[0].Tags {
			if !expectedTags[tag] {
				t.Errorf("unexpected tag: %s", tag)
			}
		}
	})
}

func TestGetAllTagemonCRs(t *testing.T) {
	t.Run("should retrieve all Tagemon CRs successfully", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)

		tagemon1 := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tagemon1",
				Namespace: "default",
			},
			Spec: v1alpha1.TagemonSpec{
				Type: "AWS/EC2",
			},
		}

		tagemon2 := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tagemon2",
				Namespace: "test-ns",
			},
			Spec: v1alpha1.TagemonSpec{
				Type: "AWS/RDS",
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(tagemon1, tagemon2).
			Build()

		monitor := &ThresholdMonitor{client: fakeClient}

		ctx := context.TODO()
		result, err := monitor.getAllTagemonCRs(ctx)

		if err != nil {
			t.Fatalf("getAllTagemonCRs failed: %v", err)
		}

		if len(result) != 2 {
			t.Errorf("expected 2 Tagemon CRs, got %d", len(result))
		}

		foundNames := make(map[string]bool)
		for _, tagemon := range result {
			foundNames[tagemon.Name] = true
		}

		if !foundNames["tagemon1"] || !foundNames["tagemon2"] {
			t.Errorf("expected to find both tagemon1 and tagemon2, got names: %v", foundNames)
		}
	})

	t.Run("should return empty list when no Tagemon CRs exist", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		monitor := &ThresholdMonitor{client: fakeClient}

		ctx := context.TODO()
		result, err := monitor.getAllTagemonCRs(ctx)

		if err != nil {
			t.Fatalf("getAllTagemonCRs failed: %v", err)
		}

		if len(result) != 0 {
			t.Errorf("expected 0 Tagemon CRs, got %d", len(result))
		}
	})

	t.Run("should handle client errors gracefully", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		monitor := &ThresholdMonitor{client: fakeClient}

		ctx := context.TODO()
		result, err := monitor.getAllTagemonCRs(ctx)

		if err == nil {
			t.Error("expected getAllTagemonCRs to fail with scheme error")
		}

		if result != nil {
			t.Errorf("expected nil result on error, got %v", result)
		}
	})
}

// =============================================================================
// TAG PARSING TESTS
// =============================================================================

func TestParseTagFromData(t *testing.T) {
	monitor := &ThresholdMonitor{}

	t.Run("should parse tag from AWS Resource Explorer format", func(t *testing.T) {
		data := []interface{}{
			map[string]interface{}{
				"Key":   "Environment",
				"Value": "production",
			},
			map[string]interface{}{
				"Key":   "MaxCPU",
				"Value": "80",
			},
		}

		result := monitor.parseTagFromData(data, "MaxCPU")

		if result != "80" {
			t.Errorf("expected tag value '80', got '%s'", result)
		}
	})

	t.Run("should parse tag from simple map format", func(t *testing.T) {
		data := map[string]interface{}{
			"Environment": "production",
			"MaxCPU":      "80",
		}

		result := monitor.parseTagFromData(data, "MaxCPU")

		if result != "80" {
			t.Errorf("expected tag value '80', got '%s'", result)
		}
	})

	t.Run("should handle nested map format", func(t *testing.T) {
		data := map[string]interface{}{
			"Tags": map[string]interface{}{
				"Environment": "production",
				"MaxCPU":      "80",
			},
		}

		result := monitor.parseTagFromData(data, "MaxCPU")

		if result != "80" {
			t.Errorf("expected tag value '80', got '%s'", result)
		}
	})

	t.Run("should handle string format", func(t *testing.T) {
		data := "some string containing MaxCPU tag"

		result := monitor.parseTagFromData(data, "MaxCPU")

		if result != "<found in JSON - parsing needed>" {
			t.Errorf("expected '<found in JSON - parsing needed>', got '%s'", result)
		}
	})

	t.Run("should return unable to parse for unknown formats", func(t *testing.T) {
		data := 12345

		result := monitor.parseTagFromData(data, "MaxCPU")

		if result != unableToParse {
			t.Errorf("expected '%s', got '%s'", unableToParse, result)
		}
	})

	t.Run("should handle direct map key match", func(t *testing.T) {
		data := map[string]interface{}{
			"Environment": "production",
			"MaxCPU":      100,
			"MinMemory":   nil,
		}

		result := monitor.parseTagFromData(data, "MaxCPU")
		if result != "100" {
			t.Errorf("expected '100', got '%s'", result)
		}

		result = monitor.parseTagFromData(data, "MinMemory")
		if result != "<nil>" {
			t.Errorf("expected '<nil>' for nil value, got '%s'", result)
		}
	})

	t.Run("should handle missing key in simple map", func(t *testing.T) {
		data := map[string]interface{}{
			"Environment": "production",
		}

		result := monitor.parseTagFromData(data, "MissingKey")

		if result != unableToParse {
			t.Errorf("expected '%s' for missing key, got '%s'", unableToParse, result)
		}
	})

	t.Run("should handle missing key in nested map", func(t *testing.T) {
		data := map[string]interface{}{
			"Tags": map[string]interface{}{
				"Environment": "production",
			},
		}

		result := monitor.parseTagFromData(data, "MissingKey")

		if result != unableToParse {
			t.Errorf("expected '%s' for missing nested key, got '%s'", unableToParse, result)
		}
	})

	t.Run("should handle invalid array format", func(t *testing.T) {
		data := []interface{}{
			map[string]interface{}{
				"InvalidKey": "InvalidValue",
			},
		}

		result := monitor.parseTagFromData(data, "AnyTag")

		if result != unableToParse {
			t.Errorf("expected '%s' for invalid array format, got '%s'", unableToParse, result)
		}
	})

	t.Run("should handle array with missing Value field", func(t *testing.T) {
		data := []interface{}{
			map[string]interface{}{
				"Key": "MaxCPU",
			},
		}

		result := monitor.parseTagFromData(data, "MaxCPU")

		if result != unableToParse {
			t.Errorf("expected '%s' for missing Value field, got '%s'", unableToParse, result)
		}
	})

	t.Run("should handle string without target tag", func(t *testing.T) {
		data := "some string without the target"

		result := monitor.parseTagFromData(data, "MissingTag")

		if result != unableToParse {
			t.Errorf("expected '%s' for string without target tag, got '%s'", unableToParse, result)
		}
	})

	t.Run("should handle empty array", func(t *testing.T) {
		data := []interface{}{}

		result := monitor.parseTagFromData(data, "AnyTag")

		if result != unableToParse {
			t.Errorf("expected '%s' for empty array, got '%s'", unableToParse, result)
		}
	})

	t.Run("should handle empty map", func(t *testing.T) {
		data := map[string]interface{}{}

		result := monitor.parseTagFromData(data, "AnyTag")

		if result != unableToParse {
			t.Errorf("expected '%s' for empty map, got '%s'", unableToParse, result)
		}
	})

	t.Run("should handle complex nested structure", func(t *testing.T) {
		data := map[string]interface{}{
			"Metadata": map[string]interface{}{
				"Tags": map[string]interface{}{
					"MaxCPU": "90",
				},
			},
		}

		result := monitor.parseTagFromData(data, "MaxCPU")

		if result != unableToParse {
			t.Errorf("expected '%s' for deeply nested tag, got '%s'", unableToParse, result)
		}
	})
}

func TestExtractTagFromDocumentString(t *testing.T) {
	monitor := &ThresholdMonitor{}

	testCases := []struct {
		name     string
		dataStr  string
		tagName  string
		expected string
	}{
		{
			name:     "simple tag extraction",
			dataStr:  "map[Key:MaxCPU Value:80]",
			tagName:  "MaxCPU",
			expected: "80",
		},
		{
			name:     "multiple tags with target tag",
			dataStr:  "map[Key:Environment Value:production] map[Key:MaxCPU Value:80] map[Key:Team Value:platform]",
			tagName:  "MaxCPU",
			expected: "80",
		},
		{
			name:     "tag with complex value",
			dataStr:  "map[Key:Description Value:Web server for production environment]",
			tagName:  "Description",
			expected: "Web server for production environment",
		},
		{
			name:     "tag with numeric value",
			dataStr:  "map[Key:Port Value:8080]",
			tagName:  "Port",
			expected: "8080",
		},
		{
			name:     "special characters in tag name",
			dataStr:  "map[Key:app.kubernetes.io/name Value:my-app]",
			tagName:  "app.kubernetes.io/name",
			expected: "my-app",
		},
		{
			name:     "missing tag",
			dataStr:  "map[Key:Environment Value:production]",
			tagName:  "MissingTag",
			expected: "<extraction failed>",
		},
		{
			name:     "empty data string",
			dataStr:  "",
			tagName:  "AnyTag",
			expected: "<extraction failed>",
		},
		{
			name:     "malformed data string",
			dataStr:  "not-a-valid-format",
			tagName:  "AnyTag",
			expected: "<extraction failed>",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := monitor.extractTagFromDocumentString(tc.dataStr, tc.tagName)

			if result != tc.expected {
				t.Errorf("expected '%s', got '%s'", tc.expected, result)
			}
		})
	}
}

// =============================================================================
// INTEGRATION TESTS
// =============================================================================

func TestRunMonitoring_Integration(t *testing.T) {
	t.Run("should handle complete monitoring flow with mock data", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)

		tagemon1 := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ec2-monitor",
				Namespace: "default",
			},
			Spec: v1alpha1.TagemonSpec{
				Type:          "AWS/EC2",
				ThresholdTags: []string{"MaxCPU", "Environment"},
			},
		}

		tagemon2 := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rds-monitor",
				Namespace: "production",
			},
			Spec: v1alpha1.TagemonSpec{
				Type:          "AWS/RDS",
				ThresholdTags: []string{"MaxConnections"},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(tagemon1, tagemon2).
			Build()

		monitor := &ThresholdMonitor{client: fakeClient}

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		tagemons, err := monitor.getAllTagemonCRs(ctx)
		if err != nil {
			t.Fatalf("failed to get Tagemon CRs: %v", err)
		}

		if len(tagemons) != 2 {
			t.Errorf("expected 2 Tagemon CRs, got %d", len(tagemons))
		}

		grouped := monitor.groupThresholdTagsByType(tagemons)
		if len(grouped) != 2 {
			t.Errorf("expected 2 grouped types, got %d", len(grouped))
		}

		groupsByType := make(map[string]ThresholdTagsByType)
		for _, group := range grouped {
			groupsByType[group.TagemonType] = group
		}

		ec2Group, hasEC2 := groupsByType["AWS/EC2"]
		if !hasEC2 {
			t.Error("expected AWS/EC2 group")
		} else {
			if len(ec2Group.Tags) != 2 {
				t.Errorf("expected 2 tags for EC2, got %d", len(ec2Group.Tags))
			}
		}

		rdsGroup, hasRDS := groupsByType["AWS/RDS"]
		if !hasRDS {
			t.Error("expected AWS/RDS group")
		} else {
			if len(rdsGroup.Tags) != 1 {
				t.Errorf("expected 1 tag for RDS, got %d", len(rdsGroup.Tags))
			}
		}
	})

	t.Run("should handle empty Tagemon list gracefully", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		monitor := &ThresholdMonitor{client: fakeClient}

		ctx := context.TODO()

		tagemons, err := monitor.getAllTagemonCRs(ctx)
		if err != nil {
			t.Fatalf("failed to get Tagemon CRs: %v", err)
		}

		if len(tagemons) != 0 {
			t.Errorf("expected 0 Tagemon CRs for empty cluster, got %d", len(tagemons))
		}

		grouped := monitor.groupThresholdTagsByType(tagemons)
		if len(grouped) != 0 {
			t.Errorf("expected 0 grouped types for empty list, got %d", len(grouped))
		}
	})

	t.Run("should handle Tagemon CRs without threshold tags", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)

		tagemon := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "no-threshold-tags",
				Namespace: "default",
			},
			Spec: v1alpha1.TagemonSpec{
				Type:          "AWS/S3",
				ThresholdTags: []string{},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(tagemon).
			Build()

		monitor := &ThresholdMonitor{client: fakeClient}

		ctx := context.TODO()

		tagemons, err := monitor.getAllTagemonCRs(ctx)
		if err != nil {
			t.Fatalf("failed to get Tagemon CRs: %v", err)
		}

		if len(tagemons) != 1 {
			t.Errorf("expected 1 Tagemon CR, got %d", len(tagemons))
		}

		grouped := monitor.groupThresholdTagsByType(tagemons)
		if len(grouped) != 0 {
			t.Errorf("expected 0 grouped types for CR without threshold tags, got %d", len(grouped))
		}
	})

	t.Run("should simulate full runMonitoring workflow without AWS calls", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)

		tagemon1 := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{Name: "with-tags", Namespace: "default"},
			Spec: v1alpha1.TagemonSpec{
				Type:          "AWS/EC2",
				ThresholdTags: []string{"MaxCPU"},
			},
		}

		tagemon2 := &v1alpha1.Tagemon{
			ObjectMeta: metav1.ObjectMeta{Name: "without-tags", Namespace: "default"},
			Spec: v1alpha1.TagemonSpec{
				Type:          "AWS/RDS",
				ThresholdTags: []string{},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(tagemon1, tagemon2).
			Build()

		monitor := &ThresholdMonitor{client: fakeClient}
		ctx := context.TODO()

		tagemons, err := monitor.getAllTagemonCRs(ctx)
		if err != nil {
			t.Fatalf("Step 1 failed: %v", err)
		}
		if len(tagemons) != 2 {
			t.Errorf("Step 1: expected 2 CRs, got %d", len(tagemons))
		}

		tagsByType := monitor.groupThresholdTagsByType(tagemons)
		if len(tagsByType) != 1 {
			t.Errorf("Step 2: expected 1 grouped type, got %d", len(tagsByType))
		}

		if len(tagsByType) > 0 {
			serviceName := monitor.extractServiceFromTagemonType(tagsByType[0].TagemonType)
			if serviceName != "ec2" {
				t.Errorf("Step 3: expected service name 'ec2', got '%s'", serviceName)
			}
		}
	})
}

// =============================================================================
// EDGE CASE TESTS
// =============================================================================

func TestEdgeCases(t *testing.T) {
	monitor := &ThresholdMonitor{}

	t.Run("should handle malformed tag array", func(t *testing.T) {
		data := []interface{}{
			"not-a-map",
			123,
			map[string]interface{}{
				"InvalidKey": "InvalidValue",
			},
		}

		result := monitor.parseTagFromData(data, "AnyTag")

		if result != unableToParse {
			t.Errorf("expected '%s' for malformed data, got '%s'", unableToParse, result)
		}
	})

	t.Run("should handle tag with nil value", func(t *testing.T) {
		data := []interface{}{
			map[string]interface{}{
				"Key":   "TestTag",
				"Value": nil,
			},
		}

		result := monitor.parseTagFromData(data, "TestTag")

		if result != "<nil>" {
			t.Errorf("expected '<nil>' for nil tag value, got '%s'", result)
		}
	})
}

func TestConcurrentAccess(t *testing.T) {
	t.Run("should handle concurrent tag grouping safely", func(t *testing.T) {
		monitor := &ThresholdMonitor{}

		tagemons := []v1alpha1.Tagemon{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "test1"},
				Spec: v1alpha1.TagemonSpec{
					Type:          "AWS/EC2",
					ThresholdTags: []string{"Tag1", "Tag2"},
				},
			},
		}

		results := make(chan []ThresholdTagsByType, 5)
		for i := 0; i < 5; i++ {
			go func() {
				results <- monitor.groupThresholdTagsByType(tagemons)
			}()
		}

		for i := 0; i < 5; i++ {
			select {
			case result := <-results:
				if len(result) != 1 {
					t.Errorf("expected 1 grouped type, got %d", len(result))
				}
				if result[0].TagemonType != "AWS/EC2" {
					t.Errorf("expected AWS/EC2 type, got %s", result[0].TagemonType)
				}
			case <-time.After(1 * time.Second):
				t.Fatal("timeout waiting for concurrent result")
			}
		}
	})
}

func TestThresholdTagsByTypeStructure(t *testing.T) {
	t.Run("should have correct struct fields", func(t *testing.T) {
		var tagsByType ThresholdTagsByType

		structType := reflect.TypeOf(tagsByType)

		expectedFields := map[string]string{
			"TagemonType": "string",
			"Tags":        "[]string",
		}

		if structType.NumField() != len(expectedFields) {
			t.Errorf("expected %d fields, got %d", len(expectedFields), structType.NumField())
		}

		for i := 0; i < structType.NumField(); i++ {
			field := structType.Field(i)
			expectedType, exists := expectedFields[field.Name]

			if !exists {
				t.Errorf("unexpected field: %s", field.Name)
				continue
			}

			if field.Type.String() != expectedType {
				t.Errorf("expected field %s to have type %s, got %s", field.Name, expectedType, field.Type.String())
			}
		}
	})

	t.Run("should initialize with zero values", func(t *testing.T) {
		var tagsByType ThresholdTagsByType

		if tagsByType.TagemonType != "" {
			t.Errorf("expected TagemonType to be empty, got '%s'", tagsByType.TagemonType)
		}

		if tagsByType.Tags != nil {
			t.Errorf("expected Tags to be nil, got %v", tagsByType.Tags)
		}
	})
}
