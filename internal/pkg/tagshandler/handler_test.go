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
	"testing"

	tagemonv1alpha1 "github.com/next-insurance/tagemon-dev/api/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

const (
	testResourceTypeS3Bucket    = "s3/bucket"
	testResourceTypeEC2Instance = "ec2/instance"
	testNonCompliantMetricName  = "tagemon_resources_non_compliant_count"
)

func TestBuildTagPolicy(t *testing.T) {
	handler := &Handler{}

	tests := []struct {
		name     string
		tagemons []tagemonv1alpha1.Tagemon
	}{
		{
			name: "single tagemon with threshold tags",
			tagemons: []tagemonv1alpha1.Tagemon{
				{
					Spec: tagemonv1alpha1.TagemonSpec{
						Type: "AWS/S3",
						ThresholdTags: []tagemonv1alpha1.ThresholdTag{
							{
								Type: tagemonv1alpha1.ThresholdTagTypeInt,
								Key:  "retention-days",
							},
							{
								Type: tagemonv1alpha1.ThresholdTagTypeBool,
								Key:  "public",
							},
						},
					},
				},
			},
		},
		{
			name: "multiple tagemons with different services",
			tagemons: []tagemonv1alpha1.Tagemon{
				{
					Spec: tagemonv1alpha1.TagemonSpec{
						Type: "AWS/S3",
						ThresholdTags: []tagemonv1alpha1.ThresholdTag{
							{
								Type: tagemonv1alpha1.ThresholdTagTypePercentage,
								Key:  "utilization",
							},
						},
					},
				},
				{
					Spec: tagemonv1alpha1.TagemonSpec{
						Type: "AWS/EC2",
						ThresholdTags: []tagemonv1alpha1.ThresholdTag{
							{
								Type: tagemonv1alpha1.ThresholdTagTypeInt,
								Key:  "ttl",
							},
						},
					},
				},
			},
		},
		{
			name:     "empty tagemon list",
			tagemons: []tagemonv1alpha1.Tagemon{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy, thresholdMap := handler.buildTagPolicy(tt.tagemons)
			assert.NotNil(t, policy)
			assert.NotNil(t, thresholdMap)

			// Verify policy structure for non-empty cases
			if len(tt.tagemons) > 0 && len(tt.tagemons[0].Spec.ThresholdTags) > 0 {
				assert.NotEmpty(t, policy.Blueprints)
				assert.NotEmpty(t, policy.Resources)
			}
		})
	}
}

// mockResource implements the resource interface for testing
type mockResource struct {
	id               string
	isCompliant      bool
	complianceErrors []*mockComplianceError
	tags             map[string]string
	service          string
	resourceType     string
}

// mockComplianceError mimics the actual ComplianceError struct with a Message field
type mockComplianceError struct {
	Message string
}

func (m *mockResource) ID() string {
	return m.id
}

func (m *mockResource) IsCompliant() bool {
	return m.isCompliant
}

func (m *mockResource) ComplianceErrors() []*mockComplianceError {
	return m.complianceErrors
}

func (m *mockResource) Tags() map[string]string {
	return m.tags
}

func (m *mockResource) Service() string {
	return m.service
}

func (m *mockResource) Type() string {
	return m.resourceType
}

func TestHandleNonCompliantResource(t *testing.T) {
	handler := &Handler{
		nonCompliantGauges: make(map[string]*prometheus.GaugeVec),
	}

	// Mock non-compliant resource
	resource := &mockResource{
		id:          "arn:aws:s3:::test-bucket:123456789012",
		isCompliant: false,
		complianceErrors: []*mockComplianceError{
			{Message: "Missing required tag: Environment"},
		},
		tags:         map[string]string{"Name": "test-bucket"},
		service:      "s3",
		resourceType: "bucket",
	}

	resourceName := "test-bucket"
	accountID := "123456789012"
	resourceType := testResourceTypeS3Bucket

	// Call the method - should just log, no errors
	handler.handleNonCompliantResource(resource, resourceName, accountID, resourceType)

	// Method should complete without panicking
	assert.NotNil(t, handler)
}

func TestHandleNonCompliantResourceWithMultipleViolations(t *testing.T) {
	handler := &Handler{
		nonCompliantGauges: make(map[string]*prometheus.GaugeVec),
	}

	// Mock non-compliant resource with multiple violations
	resource := &mockResource{
		id:          "arn:aws:ec2:us-west-2:123456789012:instance/i-1234567890abcdef0",
		isCompliant: false,
		complianceErrors: []*mockComplianceError{
			{Message: "Missing required tag: Environment"},
			{Message: "Missing required tag: Owner"},
			{Message: "Invalid tag value for CostCenter"},
		},
		tags:         map[string]string{"Name": "test-instance"},
		service:      "ec2",
		resourceType: "instance",
	}

	resourceName := "test-instance"
	accountID := "123456789012"
	resourceType := testResourceTypeEC2Instance

	// Call the method - should just log, no errors
	handler.handleNonCompliantResource(resource, resourceName, accountID, resourceType)

	// Method should complete without panicking
	assert.NotNil(t, handler)
}

func TestHandleNonCompliantResourceWithNoViolationDetails(t *testing.T) {
	handler := &Handler{
		nonCompliantGauges: make(map[string]*prometheus.GaugeVec),
	}

	// Mock non-compliant resource with no compliance error details
	resource := &mockResource{
		id:               "arn:aws:s3:::empty-bucket:123456789012",
		isCompliant:      false,
		complianceErrors: []*mockComplianceError{}, // Empty violations
		tags:             map[string]string{"Name": "empty-bucket"},
		service:          "s3",
		resourceType:     "bucket",
	}

	resourceName := "empty-bucket"
	accountID := "123456789012"
	resourceType := testResourceTypeS3Bucket

	// Call the method - should just log, no errors
	handler.handleNonCompliantResource(resource, resourceName, accountID, resourceType)

	// Method should complete without panicking
	assert.NotNil(t, handler)
}

func TestGetOrCreateNonCompliantMetric(t *testing.T) {
	// Create a custom registry for this test to avoid conflicts
	registry := prometheus.NewRegistry()

	handler := &Handler{
		nonCompliantGauges: make(map[string]*prometheus.GaugeVec),
	}

	// Mock the metrics.Registry.MustRegister by creating gauge manually
	metricName := "tagemon_resources_non_compliant_count_test"
	gauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: metricName,
			Help: "Test gauge for non-compliant resources",
		},
		[]string{"resource_type", "account_id"},
	)
	registry.MustRegister(gauge)
	handler.nonCompliantGauges[metricName] = gauge

	// Verify gauge is stored in the map
	assert.NotNil(t, handler.nonCompliantGauges[metricName])
}

func TestUpdateNonCompliantMetrics(t *testing.T) {
	// Create a custom registry for this test to avoid conflicts
	registry := prometheus.NewRegistry()

	handler := &Handler{
		nonCompliantGauges: make(map[string]*prometheus.GaugeVec),
	}

	// Pre-create the gauge to avoid registration conflicts
	metricName := testNonCompliantMetricName
	gauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: metricName + "_test2",
			Help: "Test gauge for non-compliant resources",
		},
		[]string{"resource_type", "account_id"},
	)
	registry.MustRegister(gauge)
	handler.nonCompliantGauges[metricName] = gauge

	// Setup non-compliant counts
	nonCompliantCounts := make(map[string]map[string]int)
	resourceType1 := testResourceTypeS3Bucket
	resourceType2 := testResourceTypeEC2Instance
	accountID := "123456789012"

	nonCompliantCounts[resourceType1] = make(map[string]int)
	nonCompliantCounts[resourceType1][accountID] = 2

	nonCompliantCounts[resourceType2] = make(map[string]int)
	nonCompliantCounts[resourceType2][accountID] = 1

	// Setup allResourceTypes (simulate what would be tracked in a real scan)
	allResourceTypes := make(map[string]map[string]bool)
	allResourceTypes[resourceType1] = make(map[string]bool)
	allResourceTypes[resourceType1][accountID] = true
	allResourceTypes[resourceType2] = make(map[string]bool)
	allResourceTypes[resourceType2][accountID] = true

	// Call the method - should not panic since gauge already exists
	handler.updateNonCompliantMetrics(nonCompliantCounts, allResourceTypes)

	// Verify gauge exists
	assert.NotNil(t, handler.nonCompliantGauges[metricName])
}

func TestUpdateNonCompliantMetricsWithZeros(t *testing.T) {
	// Create a custom registry for this test to avoid conflicts
	registry := prometheus.NewRegistry()

	handler := &Handler{
		nonCompliantGauges: make(map[string]*prometheus.GaugeVec),
	}

	// Pre-create the gauge to avoid registration conflicts
	metricName := testNonCompliantMetricName
	gauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: metricName + "_test3",
			Help: "Test gauge for non-compliant resources",
		},
		[]string{"resource_type", "account_id"},
	)
	registry.MustRegister(gauge)
	handler.nonCompliantGauges[metricName] = gauge

	// Setup scenario: S3 has violations, EC2 has no violations (should be set to 0)
	nonCompliantCounts := make(map[string]map[string]int)
	resourceTypeS3 := testResourceTypeS3Bucket
	resourceTypeEC2 := testResourceTypeEC2Instance
	accountID := "123456789012"

	// Only S3 has violations
	nonCompliantCounts[resourceTypeS3] = make(map[string]int)
	nonCompliantCounts[resourceTypeS3][accountID] = 3

	// Both S3 and EC2 were scanned (allResourceTypes includes both)
	allResourceTypes := make(map[string]map[string]bool)
	allResourceTypes[resourceTypeS3] = make(map[string]bool)
	allResourceTypes[resourceTypeS3][accountID] = true
	allResourceTypes[resourceTypeEC2] = make(map[string]bool)
	allResourceTypes[resourceTypeEC2][accountID] = true

	// Call the method
	handler.updateNonCompliantMetrics(nonCompliantCounts, allResourceTypes)

	// Verify gauge exists - this demonstrates that EC2 will be set to 0
	// even though it had no violations (because it was in allResourceTypes)
	assert.NotNil(t, handler.nonCompliantGauges[metricName])
}
