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
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/eliran89c/tag-patrol/pkg/cloudresource/provider/aws"
	"github.com/eliran89c/tag-patrol/pkg/patrol"
	policyTypes "github.com/eliran89c/tag-patrol/pkg/policy/types"
	tagemonv1alpha1 "github.com/next-insurance/tagemon-dev/api/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/next-insurance/tagemon-dev/internal/pkg/confighandler"
)

type Handler struct {
	client             client.Client
	metricsGauges      map[string]*prometheus.GaugeVec
	nonCompliantGauges map[string]*prometheus.GaugeVec
}

func New(k8sClient client.Client) *Handler {
	return &Handler{
		client:             k8sClient,
		metricsGauges:      make(map[string]*prometheus.GaugeVec),
		nonCompliantGauges: make(map[string]*prometheus.GaugeVec),
	}
}

// ResourceResult represents the compliance result for a single resource
type ResourceResult struct {
	ResourceARN   string            `json:"resourceArn"`
	ResourceType  string            `json:"resourceType"`
	IsCompliant   bool              `json:"isCompliant"`
	ThresholdTags map[string]string `json:"thresholdTags"`
	Violations    []string          `json:"violations,omitempty"`
	Tags          map[string]string `json:"tags,omitempty"`
}

// ComplianceReport represents the overall compliance report
type ComplianceReport struct {
	TotalResources     int              `json:"totalResources"`
	CompliantResources int              `json:"compliantResources"`
	ViolatingResources int              `json:"violatingResources"`
	ComplianceRate     float64          `json:"complianceRate"`
	Results            []ResourceResult `json:"results"`
}

// CheckCompliance reads Tagemon CRs and checks tag compliance for resources
func (h *Handler) CheckCompliance(ctx context.Context, namespace string, viewARN string, region string) (*ComplianceReport, error) {
	logger := log.FromContext(ctx)

	// Get all Tagemon CRs in the namespace
	tagemonList := &tagemonv1alpha1.TagemonList{}
	if err := h.client.List(ctx, tagemonList, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list Tagemon CRs: %w", err)
	}

	if len(tagemonList.Items) == 0 {
		logger.Info("No Tagemon CRs found in namespace", "namespace", namespace)
		return &ComplianceReport{}, nil
	}

	// Build tag policy from CRs
	policy, thresholdTagsMap := h.buildTagPolicy(tagemonList.Items)

	// Create AWS provider
	provider, err := aws.NewProvider(ctx, aws.WithViewARN(viewARN), aws.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS provider: %w", err)
	}

	// Run tag patrol
	p := patrol.New(provider, &patrol.Options{StopOnError: false, ConcurrentWorkers: 10})
	results, err := p.RunFromPolicy(ctx, policy)
	if err != nil {
		return nil, fmt.Errorf("failed to execute tag patrol: %w", err)
	}

	// Process results
	report := h.processResults(ctx, results, thresholdTagsMap)

	logger.Info("Tag compliance check completed",
		"totalResources", report.TotalResources,
		"compliantResources", report.CompliantResources,
		"complianceRate", report.ComplianceRate)

	return report, nil
}

// buildTagPolicy creates a tag-patrol policy from Tagemon CRs
func (h *Handler) buildTagPolicy(tagemons []tagemonv1alpha1.Tagemon) (*policyTypes.Policy, map[string]map[string]tagemonv1alpha1.ThresholdTagType) {
	policy := &policyTypes.Policy{
		Blueprints: make(map[string]*policyTypes.Blueprint),
		Resources:  make(map[string]map[string]*policyTypes.ResourceConfig),
	}

	// Map to track threshold tags by service and resource type
	thresholdTagsMap := make(map[string]map[string]tagemonv1alpha1.ThresholdTagType)

	for _, tagemon := range tagemons {
		if len(tagemon.Spec.ThresholdTags) == 0 {
			continue
		}

		// Parse AWS service type (e.g., "AWS/S3" -> "s3")
		serviceType := strings.ToLower(strings.TrimPrefix(tagemon.Spec.Type, "AWS/"))

		// Create threshold tags map for this service
		if thresholdTagsMap[serviceType] == nil {
			thresholdTagsMap[serviceType] = make(map[string]tagemonv1alpha1.ThresholdTagType)
		}

		// Build mandatory keys and validations from threshold tags
		var mandatoryKeys []string
		validations := make(map[string]*policyTypes.Validation)

		for _, thresholdTag := range tagemon.Spec.ThresholdTags {
			mandatoryKeys = append(mandatoryKeys, thresholdTag.Key)
			thresholdTagsMap[serviceType][thresholdTag.Key] = thresholdTag.Type

			// Create validation based on type
			validation := &policyTypes.Validation{}
			switch thresholdTag.Type {
			case tagemonv1alpha1.ThresholdTagTypeInt:
				validation.Type = "int"
			case tagemonv1alpha1.ThresholdTagTypeBool:
				validation.Type = "bool"
			case tagemonv1alpha1.ThresholdTagTypePercentage:
				validation.Type = "int"
				validation.MinValue = 0
				validation.MaxValue = 100
			}
			validations[thresholdTag.Key] = validation
		}

		// Create blueprint for this service if it has threshold tags
		if len(mandatoryKeys) > 0 {
			blueprintName := fmt.Sprintf("%s-base", serviceType)
			policy.Blueprints[blueprintName] = &policyTypes.Blueprint{
				TagPolicy: &policyTypes.TagPolicy{
					MandatoryKeys: mandatoryKeys,
					Validations:   validations,
				},
			}

			if policy.Resources[serviceType] == nil {
				policy.Resources[serviceType] = make(map[string]*policyTypes.ResourceConfig)
			}

			// Use wildcard to match all resource types for this service
			policy.Resources[serviceType]["*"] = &policyTypes.ResourceConfig{
				Extends: []string{fmt.Sprintf("blueprints.%s", blueprintName)},
			}
		}
	}

	return policy, thresholdTagsMap
}

// getOrCreateMetric gets or creates a Prometheus gauge for a threshold tag
func (h *Handler) getOrCreateMetric(tagKey string) *prometheus.GaugeVec {
	metricName := h.tagToMetricName(tagKey)

	if gauge, exists := h.metricsGauges[metricName]; exists {
		return gauge
	}

	// Create new gauge
	gauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: metricName,
			Help: fmt.Sprintf("Tagemon threshold tag value for %s", tagKey),
		},
		[]string{"resource_name", "account_id", "type"},
	)

	// Register with controller-runtime metrics
	metrics.Registry.MustRegister(gauge)
	h.metricsGauges[metricName] = gauge

	return gauge
}

func (h *Handler) getOrCreateNonCompliantMetric() *prometheus.GaugeVec {
	metricName := "tagemon_resources_non_compliant_count"

	if gauge, exists := h.nonCompliantGauges[metricName]; exists {
		return gauge
	}

	// Create new gauge
	gauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: metricName,
			Help: "Current count of non-compliant resources by type and account",
		},
		[]string{"resource_type", "account_id"},
	)

	// Register with controller-runtime metrics
	metrics.Registry.MustRegister(gauge)
	h.nonCompliantGauges[metricName] = gauge

	return gauge
}

// tagToMetricName converts a tag key to a valid Prometheus metric name
func (h *Handler) tagToMetricName(tagKey string) string {
	// Convert to lowercase and replace invalid characters with underscores
	metricName := strings.ToLower(tagKey)
	// Replace any non-alphanumeric characters (except underscores) with underscores
	reg := regexp.MustCompile(`[^a-z0-9_]`)
	metricName = reg.ReplaceAllString(metricName, "_")
	// Remove consecutive underscores
	reg = regexp.MustCompile(`_+`)
	metricName = reg.ReplaceAllString(metricName, "_")
	// Remove leading/trailing underscores
	metricName = strings.Trim(metricName, "_")

	return "tagemon_" + metricName
}

// =============================================================================
// SCHEDULER - Runs compliance checks on startup and intervals
// =============================================================================

// Scheduler implements the Runnable interface to run compliance checks after cache is ready
type Scheduler struct {
	mgr      ctrl.Manager
	handler  *Handler
	config   *confighandler.Config
	interval time.Duration
}

// Start implements the Runnable interface
func (s *Scheduler) Start(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("tagshandler-scheduler")

	// Wait for cache to sync before running initial check
	logger.Info("Waiting for cache to sync before running initial compliance check")
	if !s.mgr.GetCache().WaitForCacheSync(ctx) {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	// Run initial compliance check on startup
	logger.Info("Running initial tag compliance check on startup")
	report, err := s.handler.CheckCompliance(
		ctx,
		s.config.TagsHandler.Namespace,
		s.config.TagsHandler.ViewARN,
		s.config.TagsHandler.Region,
	)
	if err != nil {
		logger.Error(err, "Initial tag compliance check failed")
	} else {
		logger.Info("Initial tag compliance check completed",
			"totalResources", report.TotalResources,
			"complianceRate", report.ComplianceRate,
			"violatingResources", report.ViolatingResources)
	}

	// Start interval runner
	logger.Info("Starting tagshandler with interval", "interval", s.interval)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			logger.Info("Running tag compliance check")
			report, err := s.handler.CheckCompliance(
				ctx,
				s.config.TagsHandler.Namespace,
				s.config.TagsHandler.ViewARN,
				s.config.TagsHandler.Region,
			)
			if err != nil {
				logger.Error(err, "Tag compliance check failed")
			} else {
				logger.Info("Tag compliance completed",
					"totalResources", report.TotalResources,
					"complianceRate", report.ComplianceRate,
					"violatingResources", report.ViolatingResources)
			}
		case <-ctx.Done():
			return nil
		}
	}
}

// =============================================================================
// UTILITY FUNCTIONS
// =============================================================================

// extractResourceName extracts a readable resource name from tags or ARN
func (h *Handler) extractResourceName(resourceARN string, tags map[string]string) string {
	// Try to get name from common tag keys
	for _, nameKey := range []string{"Name", "name", "resource-name", "ResourceName"} {
		if name, exists := tags[nameKey]; exists && name != "" {
			return name
		}
	}

	// Fall back to extracting from ARN
	parts := strings.Split(resourceARN, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}

	// Last resort: use the full ARN
	return resourceARN
}

// extractAccountID extracts AWS account ID from ARN
func (h *Handler) extractAccountID(resourceARN string) string {
	// ARN format: arn:partition:service:region:account-id:resource-type/resource-id
	parts := strings.Split(resourceARN, ":")
	if len(parts) >= 5 {
		return parts[4]
	}
	return "unknown"
}

// processResults processes tag-patrol results, creates metrics for compliant resources and logs non-compliant ones
func (h *Handler) processResults(ctx context.Context, results []patrol.Result, thresholdTagsMap map[string]map[string]tagemonv1alpha1.ThresholdTagType) *ComplianceReport {
	logger := log.FromContext(ctx)

	totalResources := 0
	compliantResources := 0
	violatingResources := 0

	// Track non-compliant counts by resource type and account for this scan
	nonCompliantCounts := make(map[string]map[string]int)
	// Track all resource types and accounts encountered in this scan
	allResourceTypes := make(map[string]map[string]bool)

	for _, patrolResult := range results {
		if patrolResult.Error != nil {
			logger.Error(patrolResult.Error, "Error processing patrol result", "service", patrolResult.Definition.Service)
			continue
		}

		serviceType := patrolResult.Definition.Service
		thresholdTags := thresholdTagsMap[serviceType]

		// Process each resource in the result
		for _, resource := range patrolResult.Resources {
			totalResources++
			resourceName := h.extractResourceName(resource.ID(), resource.Tags())
			accountID := h.extractAccountID(resource.ID())
			resourceType := fmt.Sprintf("%s/%s", strings.ToLower(resource.Service()), strings.ToLower(resource.Type()))

			// Track this resource type and account combination
			if allResourceTypes[resourceType] == nil {
				allResourceTypes[resourceType] = make(map[string]bool)
			}
			allResourceTypes[resourceType][accountID] = true

			if resource.IsCompliant() {
				compliantResources++
				h.createMetricsForResource(resource, thresholdTags, resourceName, accountID, resourceType)
			} else {
				violatingResources++
				h.handleNonCompliantResource(resource, resourceName, accountID, resourceType)

				if nonCompliantCounts[resourceType] == nil {
					nonCompliantCounts[resourceType] = make(map[string]int)
				}
				nonCompliantCounts[resourceType][accountID]++
			}
		}
	}

	h.updateNonCompliantMetrics(nonCompliantCounts, allResourceTypes)

	complianceRate := float64(0)
	if totalResources > 0 {
		complianceRate = float64(compliantResources) / float64(totalResources) * 100
	}

	return &ComplianceReport{
		TotalResources:     totalResources,
		CompliantResources: compliantResources,
		ViolatingResources: violatingResources,
		ComplianceRate:     complianceRate,
		Results:            nil,
	}
}

// createMetricsForResource creates Prometheus metrics for compliant resources
func (h *Handler) createMetricsForResource(resource interface{}, thresholdTags map[string]tagemonv1alpha1.ThresholdTagType, resourceName, accountID, resourceType string) {
	type CloudResource interface {
		ID() string
		Tags() map[string]string
	}

	r, ok := resource.(CloudResource)
	if !ok {
		return
	}

	tags := r.Tags()

	for tagKey, tagType := range thresholdTags {
		if tagValue, exists := tags[tagKey]; exists && tagValue != "" {
			gauge := h.getOrCreateMetric(tagKey)

			// Convert tag value to float64 based on type
			var value float64
			var err error

			switch tagType {
			case tagemonv1alpha1.ThresholdTagTypeInt, tagemonv1alpha1.ThresholdTagTypePercentage:
				value, err = strconv.ParseFloat(tagValue, 64)
			case tagemonv1alpha1.ThresholdTagTypeBool:
				if strings.ToLower(tagValue) == "true" {
					value = 1
				} else {
					value = 0
				}
			}

			if err == nil {
				gauge.WithLabelValues(resourceName, accountID, resourceType).Set(value)
			}
		}
	}
}

func (h *Handler) handleNonCompliantResource(resource interface{}, resourceName, accountID, resourceType string) {
	resourceValue := reflect.ValueOf(resource)

	idMethod := resourceValue.MethodByName("ID")
	if !idMethod.IsValid() {
		return
	}

	complianceErrorsMethod := resourceValue.MethodByName("ComplianceErrors")
	if !complianceErrorsMethod.IsValid() {
		return
	}

	idResults := idMethod.Call(nil)
	if len(idResults) == 0 {
		return
	}
	resourceARN := idResults[0].String()

	complianceErrorsResults := complianceErrorsMethod.Call(nil)
	var violations []string

	if len(complianceErrorsResults) > 0 {
		complianceErrorsValue := complianceErrorsResults[0]

		if complianceErrorsValue.Kind() == reflect.Slice && complianceErrorsValue.Len() > 0 {
			for i := 0; i < complianceErrorsValue.Len(); i++ {
				errorValue := complianceErrorsValue.Index(i)

				var errorMsg string
				if errorValue.Kind() == reflect.Ptr {
					elem := errorValue.Elem()
					if elem.Kind() == reflect.Struct {
						messageField := elem.FieldByName("Message")
						if messageField.IsValid() && messageField.CanInterface() {
							errorMsg = messageField.String()
						}
					}
				}

				if errorMsg != "" {
					violations = append(violations, errorMsg)
				} else {
					violations = append(violations, "Tag compliance violation")
				}
			}
		}
	}

	if len(violations) == 0 {
		violations = append(violations, "Tag compliance violation detected")
	}

	log.FromContext(context.Background()).Info("Non-compliant resource detected",
		"resource", resourceName,
		"type", resourceType,
		"arn", resourceARN,
		"account", accountID,
		"violations", violations)
}

func (h *Handler) updateNonCompliantMetrics(nonCompliantCounts map[string]map[string]int, allResourceTypes map[string]map[string]bool) {
	gauge := h.getOrCreateNonCompliantMetric()

	gauge.Reset()

	for resourceType, accounts := range allResourceTypes {
		for accountID := range accounts {
			count := 0
			if nonCompliantCounts[resourceType] != nil {
				count = nonCompliantCounts[resourceType][accountID]
			}
			gauge.WithLabelValues(resourceType, accountID).Set(float64(count))
		}
	}
}
