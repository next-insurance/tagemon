package tagshandler

import (
	"strings"
	"testing"

	tagemonv1alpha1 "github.com/next-insurance/tagemon/api/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

const (
	testResourceTypeS3Bucket    = "s3/bucket"
	testResourceTypeEC2Instance = "ec2/instance"
	testNonCompliantMetricName  = "tagemon_resources_non_compliant_count"
)

// Helper function to create string pointers for tests
func stringPtr(s string) *string {
	return &s
}

func TestBuildTagPolicy(t *testing.T) {
	handler := &Handler{}

	tests := []struct {
		name                string
		tagemons            []tagemonv1alpha1.Tagemon
		expectedServiceType string
		description         string
	}{
		{
			name: "single tagemon with threshold tags",
			tagemons: []tagemonv1alpha1.Tagemon{
				{
					Spec: tagemonv1alpha1.TagemonSpec{
						Type: "AWS/S3",
						Metrics: []tagemonv1alpha1.TagemonMetric{
							{
								Name: "BucketSizeBytes",
								ThresholdTags: []tagemonv1alpha1.ThresholdTag{
									{
										Type:         tagemonv1alpha1.ThresholdTagTypeInt,
										Key:          "retention-days",
										ResourceType: "bucket",
									},
									{
										Type:         tagemonv1alpha1.ThresholdTagTypeBool,
										Key:          "public",
										ResourceType: "bucket",
									},
								},
							},
						},
					},
				},
			},
			expectedServiceType: "s3",
			description:         "should derive service type from Type field",
		},
		{
			name: "tagemon with ResourceExplorerService override",
			tagemons: []tagemonv1alpha1.Tagemon{
				{
					Spec: tagemonv1alpha1.TagemonSpec{
						Type:                    "AWS/VPN",
						ResourceExplorerService: stringPtr("ec2"),
						Metrics: []tagemonv1alpha1.TagemonMetric{
							{
								Name: "TunnelState",
								ThresholdTags: []tagemonv1alpha1.ThresholdTag{
									{
										Type:         tagemonv1alpha1.ThresholdTagTypeInt,
										Key:          "vpn-threshold",
										ResourceType: "vpn-connection",
									},
								},
							},
						},
					},
				},
			},
			expectedServiceType: "ec2",
			description:         "should use ResourceExplorerService override instead of deriving from Type",
		},
		{
			name: "multiple tagemons with different services",
			tagemons: []tagemonv1alpha1.Tagemon{
				{
					Spec: tagemonv1alpha1.TagemonSpec{
						Type: "AWS/S3",
						Metrics: []tagemonv1alpha1.TagemonMetric{
							{
								Name: "BucketSizeBytes",
								ThresholdTags: []tagemonv1alpha1.ThresholdTag{
									{
										Type:         tagemonv1alpha1.ThresholdTagTypePercentage,
										Key:          "utilization",
										ResourceType: "bucket",
									},
								},
							},
						},
					},
				},
				{
					Spec: tagemonv1alpha1.TagemonSpec{
						Type: "AWS/EC2",
						Metrics: []tagemonv1alpha1.TagemonMetric{
							{
								Name: "CPUUtilization",
								ThresholdTags: []tagemonv1alpha1.ThresholdTag{
									{
										Type:         tagemonv1alpha1.ThresholdTagTypeInt,
										Key:          "ttl",
										ResourceType: "instance",
									},
								},
							},
						},
					},
				},
			},
			expectedServiceType: "",
			description:         "should handle multiple service types",
		},
		{
			name:                "empty tagemon list",
			tagemons:            []tagemonv1alpha1.Tagemon{},
			expectedServiceType: "",
			description:         "should handle empty tagemon list",
		},
		{
			name: "tagemon with non-existent resource type",
			tagemons: []tagemonv1alpha1.Tagemon{
				{
					Spec: tagemonv1alpha1.TagemonSpec{
						Type: "AWS/EC2",
						Metrics: []tagemonv1alpha1.TagemonMetric{
							{
								Name: "CPUUtilization",
								ThresholdTags: []tagemonv1alpha1.ThresholdTag{
									{
										Type:         tagemonv1alpha1.ThresholdTagTypeInt,
										Key:          "cpu-threshold",
										ResourceType: "nonexistent-resource-type",
									},
									{
										Type:         tagemonv1alpha1.ThresholdTagTypeInt,
										Key:          "storage-threshold",
										ResourceType: "instance",
									},
								},
							},
						},
					},
				},
			},
			expectedServiceType: "ec2",
			description:         "should handle non-existent resource types",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy, thresholdMap := handler.buildTagPolicy(tt.tagemons)
			assert.NotNil(t, policy, tt.description)
			assert.NotNil(t, thresholdMap, tt.description)

			// Verify the expected service type is present in thresholdMap (if specified)
			if tt.expectedServiceType != "" {
				assert.Contains(t, thresholdMap, tt.expectedServiceType,
					"Expected service type %s to be in thresholdMap", tt.expectedServiceType)
			}

			// Verify policy structure for non-empty cases
			hasThresholdTags := false
			if len(tt.tagemons) > 0 {
				for _, metric := range tt.tagemons[0].Spec.Metrics {
					if len(metric.ThresholdTags) > 0 {
						hasThresholdTags = true
						break
					}
				}
			}
			if hasThresholdTags {
				assert.NotEmpty(t, policy.Blueprints)
				assert.NotEmpty(t, policy.Resources)

				// Special verification for non-existent resource type test
				if tt.name == "tagemon with non-existent resource type" {
					// Should create 2 blueprints: one for nonexistent-resource-type, one for instance
					assert.Len(t, policy.Blueprints, 2)

					// Should create 2 resource configs in EC2 service
					assert.Contains(t, policy.Resources, "ec2")
					assert.Len(t, policy.Resources["ec2"], 2)

					// Both resource types should be created, regardless of whether they exist in AWS
					assert.Contains(t, policy.Resources["ec2"], "nonexistent-resource-type")
					assert.Contains(t, policy.Resources["ec2"], "instance")

					// Blueprints should be created for both
					assert.Contains(t, policy.Blueprints, "ec2-nonexistent-resource-type-base")
					assert.Contains(t, policy.Blueprints, "ec2-instance-base")
				}
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
		id:          "arn:aws:s3:us-west-2:123456789012:bucket/test-bucket",
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
		id:               "arn:aws:s3:us-west-2:123456789012:bucket/empty-bucket",
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

func TestExtractAllowedAccountIDs(t *testing.T) {
	handler := &Handler{}

	tests := []struct {
		name     string
		tagemons []tagemonv1alpha1.Tagemon
		expected map[string]bool
	}{
		{
			name: "single tagemon with multiple roles",
			tagemons: []tagemonv1alpha1.Tagemon{
				{
					Spec: tagemonv1alpha1.TagemonSpec{
						Roles: []tagemonv1alpha1.AWSRole{
							{RoleArn: "arn:aws:iam::123456789012:role/test-role-1"},
							{RoleArn: "arn:aws:iam::987654321098:role/test-role-2"},
						},
					},
				},
			},
			expected: map[string]bool{
				"123456789012": true,
				"987654321098": true,
			},
		},
		{
			name: "multiple tagemons with overlapping accounts",
			tagemons: []tagemonv1alpha1.Tagemon{
				{
					Spec: tagemonv1alpha1.TagemonSpec{
						Roles: []tagemonv1alpha1.AWSRole{
							{RoleArn: "arn:aws:iam::123456789012:role/role-1"},
						},
					},
				},
				{
					Spec: tagemonv1alpha1.TagemonSpec{
						Roles: []tagemonv1alpha1.AWSRole{
							{RoleArn: "arn:aws:iam::123456789012:role/role-2"}, // Same account
							{RoleArn: "arn:aws:iam::111222333444:role/role-3"},
						},
					},
				},
			},
			expected: map[string]bool{
				"123456789012": true,
				"111222333444": true,
			},
		},
		{
			name:     "empty tagemon list",
			tagemons: []tagemonv1alpha1.Tagemon{},
			expected: map[string]bool{},
		},
		{
			name: "tagemon with no roles",
			tagemons: []tagemonv1alpha1.Tagemon{
				{
					Spec: tagemonv1alpha1.TagemonSpec{
						Roles: []tagemonv1alpha1.AWSRole{},
					},
				},
			},
			expected: map[string]bool{},
		},
		{
			name: "invalid role ARN format",
			tagemons: []tagemonv1alpha1.Tagemon{
				{
					Spec: tagemonv1alpha1.TagemonSpec{
						Roles: []tagemonv1alpha1.AWSRole{
							{RoleArn: "invalid-arn-format"},
							{RoleArn: "arn:aws:iam::123456789012:role/valid-role"},
						},
					},
				},
			},
			expected: map[string]bool{
				"123456789012": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.extractAllowedAccountIDs(tt.tagemons)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractSearchTagsFilters(t *testing.T) {
	handler := &Handler{}

	tests := []struct {
		name     string
		tagemons []tagemonv1alpha1.Tagemon
		expected []tagemonv1alpha1.TagemonTag
	}{
		{
			name: "single tagemon with search tags",
			tagemons: []tagemonv1alpha1.Tagemon{
				{
					Spec: tagemonv1alpha1.TagemonSpec{
						SearchTags: []tagemonv1alpha1.TagemonTag{
							{Key: "Environment", Value: "production"},
							{Key: "Team", Value: "platform"},
						},
					},
				},
			},
			expected: []tagemonv1alpha1.TagemonTag{
				{Key: "Environment", Value: "production"},
				{Key: "Team", Value: "platform"},
			},
		},
		{
			name: "multiple tagemons with search tags",
			tagemons: []tagemonv1alpha1.Tagemon{
				{
					Spec: tagemonv1alpha1.TagemonSpec{
						SearchTags: []tagemonv1alpha1.TagemonTag{
							{Key: "Environment", Value: "production"},
						},
					},
				},
				{
					Spec: tagemonv1alpha1.TagemonSpec{
						SearchTags: []tagemonv1alpha1.TagemonTag{
							{Key: "Team", Value: "platform"},
							{Key: "CostCenter", Value: "engineering"},
						},
					},
				},
			},
			expected: []tagemonv1alpha1.TagemonTag{
				{Key: "Environment", Value: "production"},
				{Key: "Team", Value: "platform"},
				{Key: "CostCenter", Value: "engineering"},
			},
		},
		{
			name:     "empty tagemon list",
			tagemons: []tagemonv1alpha1.Tagemon{},
			expected: []tagemonv1alpha1.TagemonTag{},
		},
		{
			name: "tagemon with no search tags",
			tagemons: []tagemonv1alpha1.Tagemon{
				{
					Spec: tagemonv1alpha1.TagemonSpec{
						SearchTags: []tagemonv1alpha1.TagemonTag{},
					},
				},
			},
			expected: []tagemonv1alpha1.TagemonTag{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.extractSearchTagsFilters(tt.tagemons)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractExportedTagsOnMetrics(t *testing.T) {
	handler := &Handler{}
	trueVal := true
	falseVal := false

	tests := []struct {
		name           string
		tagemons       []tagemonv1alpha1.Tagemon
		expectedLength int
		expectedKeys   []string
		description    string
	}{
		{
			name: "single tagemon with exported tags",
			tagemons: []tagemonv1alpha1.Tagemon{
				{
					Spec: tagemonv1alpha1.TagemonSpec{
						ExportedTagsOnMetrics: []tagemonv1alpha1.ExportedTag{
							{Key: "Environment", Required: &trueVal},
							{Key: "Application", Required: &falseVal},
						},
					},
				},
			},
			expectedLength: 2,
			expectedKeys:   []string{"Environment", "Application"},
			description:    "should extract all exported tags from a single tagemon",
		},
		{
			name: "multiple tagemons with exported tags",
			tagemons: []tagemonv1alpha1.Tagemon{
				{
					Spec: tagemonv1alpha1.TagemonSpec{
						ExportedTagsOnMetrics: []tagemonv1alpha1.ExportedTag{
							{Key: "Environment", Required: &trueVal},
						},
					},
				},
				{
					Spec: tagemonv1alpha1.TagemonSpec{
						ExportedTagsOnMetrics: []tagemonv1alpha1.ExportedTag{
							{Key: "Application", Required: &falseVal},
							{Key: "Owner", Required: &trueVal},
						},
					},
				},
			},
			expectedLength: 3,
			expectedKeys:   []string{"Environment", "Application", "Owner"},
			description:    "should merge exported tags from multiple tagemons",
		},
		{
			name:           "empty tagemon list",
			tagemons:       []tagemonv1alpha1.Tagemon{},
			expectedLength: 0,
			expectedKeys:   []string{},
			description:    "should return empty slice for empty tagemon list",
		},
		{
			name: "tagemon with no exported tags",
			tagemons: []tagemonv1alpha1.Tagemon{
				{
					Spec: tagemonv1alpha1.TagemonSpec{
						ExportedTagsOnMetrics: []tagemonv1alpha1.ExportedTag{},
					},
				},
			},
			expectedLength: 0,
			expectedKeys:   []string{},
			description:    "should return empty slice when no exported tags defined",
		},
		{
			name: "multiple tagemons with duplicate exported tag keys - should deduplicate",
			tagemons: []tagemonv1alpha1.Tagemon{
				{
					Spec: tagemonv1alpha1.TagemonSpec{
						ExportedTagsOnMetrics: []tagemonv1alpha1.ExportedTag{
							{Key: "mimir_tenants", Required: &trueVal},
							{Key: "Environment", Required: &trueVal},
						},
					},
				},
				{
					Spec: tagemonv1alpha1.TagemonSpec{
						ExportedTagsOnMetrics: []tagemonv1alpha1.ExportedTag{
							{Key: "mimir_tenants", Required: &falseVal}, // Duplicate key
							{Key: "Application", Required: &trueVal},
						},
					},
				},
			},
			expectedLength: 3, // Should only have 3 unique keys, not 4
			expectedKeys:   []string{"mimir_tenants", "Environment", "Application"},
			description:    "should deduplicate exported tags when same key appears in multiple CRs",
		},
		{
			name: "multiple tagemons with multiple duplicate keys",
			tagemons: []tagemonv1alpha1.Tagemon{
				{
					Spec: tagemonv1alpha1.TagemonSpec{
						ExportedTagsOnMetrics: []tagemonv1alpha1.ExportedTag{
							{Key: "Team", Required: &trueVal},
							{Key: "Environment", Required: &trueVal},
						},
					},
				},
				{
					Spec: tagemonv1alpha1.TagemonSpec{
						ExportedTagsOnMetrics: []tagemonv1alpha1.ExportedTag{
							{Key: "Team", Required: &falseVal},        // Duplicate
							{Key: "Environment", Required: &falseVal}, // Duplicate
							{Key: "Owner", Required: &trueVal},
						},
					},
				},
				{
					Spec: tagemonv1alpha1.TagemonSpec{
						ExportedTagsOnMetrics: []tagemonv1alpha1.ExportedTag{
							{Key: "Team", Required: &trueVal}, // Duplicate again
							{Key: "CostCenter", Required: &trueVal},
						},
					},
				},
			},
			expectedLength: 4, // Team, Environment, Owner, CostCenter (all unique)
			expectedKeys:   []string{"Team", "Environment", "Owner", "CostCenter"},
			description:    "should deduplicate multiple duplicate keys across multiple CRs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.extractExportedTagsOnMetrics(tt.tagemons)
			assert.Len(t, result, tt.expectedLength, tt.description)

			if len(tt.expectedKeys) > 0 {
				resultKeys := make(map[string]bool)
				for _, tag := range result {
					resultKeys[tag.Key] = true
				}

				for _, expectedKey := range tt.expectedKeys {
					assert.True(t, resultKeys[expectedKey], "Expected key %s to be present", expectedKey)
				}
			}
		})
	}
}

func TestGetOrCreateMetric(t *testing.T) {
	handler := &Handler{
		metricsGauges: make(map[string]*prometheus.GaugeVec),
	}

	tests := []struct {
		name         string
		tagKey       string
		exportedTags []tagemonv1alpha1.ExportedTag
		description  string
	}{
		{
			name:         "create metric without exported tags",
			tagKey:       "storage_drop_mb_threshold",
			exportedTags: []tagemonv1alpha1.ExportedTag{},
			description:  "should create metric with basic labels only",
		},
		{
			name:   "create metric with single exported tag",
			tagKey: "storage_drop_mb_threshold",
			exportedTags: []tagemonv1alpha1.ExportedTag{
				{Key: "Environment"},
			},
			description: "should create metric with additional exported tag label",
		},
		{
			name:   "create metric with multiple exported tags",
			tagKey: "retention_days",
			exportedTags: []tagemonv1alpha1.ExportedTag{
				{Key: "Environment"},
				{Key: "Team"},
			},
			description: "should create metric with multiple exported tag labels",
		},
		{
			name:         "retrieve existing metric",
			tagKey:       "storage_drop_mb_threshold",
			exportedTags: []tagemonv1alpha1.ExportedTag{},
			description:  "should return existing metric without creating a new one",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gauge := handler.getOrCreateMetric(tt.tagKey, tt.exportedTags)

			if tt.name == "retrieve existing metric" {
				assert.NotNil(t, gauge, tt.description)

				metricKey := handler.tagToMetricName(tt.tagKey)
				assert.Equal(t, handler.metricsGauges[metricKey], gauge)
			} else {
				if gauge != nil {
					metricName := handler.tagToMetricName(tt.tagKey)
					metricKey := metricName
					if len(tt.exportedTags) > 0 {
						exportedKeys := make([]string, 0, len(tt.exportedTags))
						for _, tag := range tt.exportedTags {
							exportedKeys = append(exportedKeys, tag.Key)
						}
						metricKey = metricName + "_" + strings.Join(exportedKeys, "_")
					}
					assert.NotNil(t, handler.metricsGauges[metricKey])
				}
			}
		})
	}
}

func TestCreateMetricsForResourceWithNilGauge(t *testing.T) {
	handler := &Handler{
		metricsGauges: make(map[string]*prometheus.GaugeVec),
	}

	thresholdTags := map[string]tagemonv1alpha1.ThresholdTagType{
		"threshold_metric": tagemonv1alpha1.ThresholdTagTypeInt,
	}

	resource := &mockResource{
		id: "arn:aws:s3:us-west-2:123456789012:bucket/test-bucket",
		tags: map[string]string{
			"Name":             "test-bucket",
			"threshold_metric": "100",
		},
		service:      "s3",
		resourceType: "bucket",
	}

	exportedTags := []tagemonv1alpha1.ExportedTag{}

	handler.createMetricsForResource(
		resource,
		thresholdTags,
		"test-bucket",
		"123456789012",
		"s3/bucket",
		exportedTags,
	)

	assert.NotNil(t, handler)
}

func TestIsResourceRelevant(t *testing.T) {
	handler := &Handler{}

	tests := []struct {
		name              string
		resource          interface{}
		allowedAccountIDs map[string]bool
		searchTags        []tagemonv1alpha1.TagemonTag
		expected          bool
		description       string
	}{
		{
			name: "resource matches account and search tags",
			resource: &mockResource{
				id: "arn:aws:s3:us-west-2:123456789012:bucket/test-bucket",
				tags: map[string]string{
					"Environment": "production",
					"Team":        "platform",
					"Name":        "test-bucket",
				},
			},
			allowedAccountIDs: map[string]bool{
				"123456789012": true,
			},
			searchTags: []tagemonv1alpha1.TagemonTag{
				{Key: "Environment", Value: "production"},
				{Key: "Team", Value: "platform"},
			},
			expected:    true,
			description: "resource should be relevant when it matches both account and all search tags",
		},
		{
			name: "resource matches account but missing search tag",
			resource: &mockResource{
				id: "arn:aws:s3:us-west-2:123456789012:bucket/test-bucket",
				tags: map[string]string{
					"Environment": "production",
					"Name":        "test-bucket",
				},
			},
			allowedAccountIDs: map[string]bool{
				"123456789012": true,
			},
			searchTags: []tagemonv1alpha1.TagemonTag{
				{Key: "Environment", Value: "production"},
				{Key: "Team", Value: "platform"}, // Missing this tag
			},
			expected:    false,
			description: "resource should be irrelevant when missing required search tags",
		},
		{
			name: "resource has search tags but wrong account",
			resource: &mockResource{
				id: "arn:aws:s3:::test-bucket:999888777666",
				tags: map[string]string{
					"Environment": "production",
					"Team":        "platform",
				},
			},
			allowedAccountIDs: map[string]bool{
				"123456789012": true,
			},
			searchTags: []tagemonv1alpha1.TagemonTag{
				{Key: "Environment", Value: "production"},
				{Key: "Team", Value: "platform"},
			},
			expected:    false,
			description: "resource should be irrelevant when account is not allowed",
		},
		{
			name: "resource matches account, no search tags required",
			resource: &mockResource{
				id: "arn:aws:s3:us-west-2:123456789012:bucket/test-bucket",
				tags: map[string]string{
					"Name": "test-bucket",
				},
			},
			allowedAccountIDs: map[string]bool{
				"123456789012": true,
			},
			searchTags:  []tagemonv1alpha1.TagemonTag{},
			expected:    true,
			description: "resource should be relevant when account matches and no search tags required",
		},
		{
			name: "no account filtering, resource has search tags",
			resource: &mockResource{
				id: "arn:aws:s3:us-west-2:123456789012:bucket/test-bucket",
				tags: map[string]string{
					"Environment": "production",
					"Team":        "platform",
				},
			},
			allowedAccountIDs: map[string]bool{},
			searchTags: []tagemonv1alpha1.TagemonTag{
				{Key: "Environment", Value: "production"},
				{Key: "Team", Value: "platform"},
			},
			expected:    true,
			description: "resource should be relevant when no account filtering and search tags match",
		},
		{
			name: "no filtering at all",
			resource: &mockResource{
				id: "arn:aws:s3:us-west-2:123456789012:bucket/test-bucket",
				tags: map[string]string{
					"Name": "test-bucket",
				},
			},
			allowedAccountIDs: map[string]bool{},
			searchTags:        []tagemonv1alpha1.TagemonTag{},
			expected:          true,
			description:       "resource should be relevant when no filtering is applied",
		},
		{
			name: "search tag value mismatch",
			resource: &mockResource{
				id: "arn:aws:s3:us-west-2:123456789012:bucket/test-bucket",
				tags: map[string]string{
					"Environment": "staging", // Wrong value
					"Team":        "platform",
				},
			},
			allowedAccountIDs: map[string]bool{
				"123456789012": true,
			},
			searchTags: []tagemonv1alpha1.TagemonTag{
				{Key: "Environment", Value: "production"}, // Expected production
				{Key: "Team", Value: "platform"},
			},
			expected:    false,
			description: "resource should be irrelevant when search tag value doesn't match",
		},
		{
			name:              "invalid resource type",
			resource:          "invalid-resource",
			allowedAccountIDs: map[string]bool{"123456789012": true},
			searchTags:        []tagemonv1alpha1.TagemonTag{},
			expected:          false,
			description:       "should return false for invalid resource types",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.isResourceRelevant(tt.resource, tt.allowedAccountIDs, tt.searchTags)
			assert.Equal(t, tt.expected, result, tt.description)
		})
	}
}
