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
	tagemonv1alpha1 "github.com/next-insurance/tagemon/api/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/next-insurance/tagemon/internal/confighandler"
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

type ResourceResult struct {
	ResourceARN   string            `json:"resourceArn"`
	ResourceType  string            `json:"resourceType"`
	IsCompliant   bool              `json:"isCompliant"`
	ThresholdTags map[string]string `json:"thresholdTags"`
	Violations    []string          `json:"violations,omitempty"`
	Tags          map[string]string `json:"tags,omitempty"`
}

type ComplianceReport struct {
	TotalResources     int              `json:"totalResources"`
	CompliantResources int              `json:"compliantResources"`
	ViolatingResources int              `json:"violatingResources"`
	ComplianceRate     float64          `json:"complianceRate"`
	Results            []ResourceResult `json:"results"`
}

func (h *Handler) CheckCompliance(ctx context.Context, namespace string, viewARN string, region string) (*ComplianceReport, error) {
	logger := log.FromContext(ctx)

	tagemonList := &tagemonv1alpha1.TagemonList{}
	if err := h.client.List(ctx, tagemonList, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list Tagemon CRs: %w", err)
	}

	if len(tagemonList.Items) == 0 {
		logger.Info("No Tagemon CRs found in namespace", "namespace", namespace)
		return &ComplianceReport{}, nil
	}

	policy, thresholdTagsMap := h.buildTagPolicy(tagemonList.Items)
	allowedAccountIDs := h.extractAllowedAccountIDs(tagemonList.Items)
	searchTagsFilters := h.extractSearchTagsFilters(tagemonList.Items)
	exportedTagsOnMetrics := h.extractExportedTagsOnMetrics(tagemonList.Items)

	if len(allowedAccountIDs) > 0 {
		accountList := make([]string, 0, len(allowedAccountIDs))
		for accountID := range allowedAccountIDs {
			accountList = append(accountList, accountID)
		}
		logger.Info("Filtering resources by allowed accounts", "accounts", accountList)
	}

	if len(searchTagsFilters) > 0 {
		logger.Info("Filtering resources by search tags", "searchTagsCount", len(searchTagsFilters))
		for _, tag := range searchTagsFilters {
			logger.V(1).Info("Search tag filter", "key", tag.Key, "value", tag.Value)
		}
	}

	provider, err := aws.NewProvider(ctx, aws.WithViewARN(viewARN), aws.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS provider: %w", err)
	}

	p := patrol.New(provider, &patrol.Options{StopOnError: false, ConcurrentWorkers: 10})
	results, err := p.RunFromPolicy(ctx, policy)
	if err != nil {
		return nil, fmt.Errorf("failed to execute tag patrol: %w", err)
	}

	report := h.processResults(ctx, results, thresholdTagsMap, allowedAccountIDs, searchTagsFilters, exportedTagsOnMetrics)

	logger.Info("Tag compliance check completed",
		"totalResources", report.TotalResources,
		"compliantResources", report.CompliantResources,
		"violatingResources", report.ViolatingResources,
		"complianceRate", report.ComplianceRate,
		"filteredByAccounts", len(allowedAccountIDs) > 0,
		"filteredBySearchTags", len(searchTagsFilters) > 0)

	return report, nil
}

func (h *Handler) buildTagPolicy(tagemons []tagemonv1alpha1.Tagemon) (*policyTypes.Policy, map[string]map[string]tagemonv1alpha1.ThresholdTagType) {
	policy := &policyTypes.Policy{
		Blueprints: make(map[string]*policyTypes.Blueprint),
		Resources:  make(map[string]map[string]*policyTypes.ResourceConfig),
	}

	thresholdTagsMap := make(map[string]map[string]tagemonv1alpha1.ThresholdTagType)

	for _, tagemon := range tagemons {
		serviceType := strings.ToLower(strings.TrimPrefix(tagemon.Spec.Type, "AWS/"))

		if thresholdTagsMap[serviceType] == nil {
			thresholdTagsMap[serviceType] = make(map[string]tagemonv1alpha1.ThresholdTagType)
		}

		var allThresholdTags []tagemonv1alpha1.ThresholdTag
		for _, metric := range tagemon.Spec.Metrics {
			allThresholdTags = append(allThresholdTags, metric.ThresholdTags...)
		}

		if len(allThresholdTags) == 0 {
			continue
		}

		resourceTypeGroups := make(map[string][]tagemonv1alpha1.ThresholdTag)
		for _, thresholdTag := range allThresholdTags {
			resourceType := thresholdTag.ResourceType
			resourceTypeGroups[resourceType] = append(resourceTypeGroups[resourceType], thresholdTag)
			thresholdTagsMap[serviceType][thresholdTag.Key] = thresholdTag.Type
		}

		for resourceType, thresholdTags := range resourceTypeGroups {
			var mandatoryKeys []string
			validations := make(map[string]*policyTypes.Validation)

			mandatoryKeys = append(mandatoryKeys, "Name")

			for _, exportedTag := range tagemon.Spec.ExportedTagsOnMetrics {
				isRequired := false
				if exportedTag.Required != nil {
					isRequired = *exportedTag.Required
				}

				if isRequired {
					mandatoryKeys = append(mandatoryKeys, exportedTag.Key)
				}
			}

			for _, thresholdTag := range thresholdTags {
				isRequired := true
				if thresholdTag.Required != nil {
					isRequired = *thresholdTag.Required
				}

				if isRequired {
					mandatoryKeys = append(mandatoryKeys, thresholdTag.Key)
				}

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

			if len(mandatoryKeys) > 0 {
				blueprintName := fmt.Sprintf("%s-%s-base", serviceType, resourceType)

				policy.Blueprints[blueprintName] = &policyTypes.Blueprint{
					TagPolicy: &policyTypes.TagPolicy{
						MandatoryKeys: mandatoryKeys,
						Validations:   validations,
					},
				}

				if policy.Resources[serviceType] == nil {
					policy.Resources[serviceType] = make(map[string]*policyTypes.ResourceConfig)
				}

				policy.Resources[serviceType][resourceType] = &policyTypes.ResourceConfig{
					Extends: []string{fmt.Sprintf("blueprints.%s", blueprintName)},
				}
			}
		}
	}

	return policy, thresholdTagsMap
}

func (h *Handler) extractAllowedAccountIDs(tagemons []tagemonv1alpha1.Tagemon) map[string]bool {
	allowedAccountIDs := make(map[string]bool)

	for _, tagemon := range tagemons {
		for _, role := range tagemon.Spec.Roles {
			// Extract account ID from role ARN: arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME
			parts := strings.Split(role.RoleArn, ":")
			if len(parts) >= 5 {
				accountID := parts[4]
				allowedAccountIDs[accountID] = true
			}
		}
	}

	return allowedAccountIDs
}

func (h *Handler) extractSearchTagsFilters(tagemons []tagemonv1alpha1.Tagemon) []tagemonv1alpha1.TagemonTag {
	searchTags := make([]tagemonv1alpha1.TagemonTag, 0)

	for _, tagemon := range tagemons {
		searchTags = append(searchTags, tagemon.Spec.SearchTags...)
	}

	return searchTags
}

func (h *Handler) extractExportedTagsOnMetrics(tagemons []tagemonv1alpha1.Tagemon) []tagemonv1alpha1.ExportedTag {
	uniqueTagsMap := make(map[string]tagemonv1alpha1.ExportedTag)

	for _, tagemon := range tagemons {
		for _, exportedTag := range tagemon.Spec.ExportedTagsOnMetrics {
			if _, exists := uniqueTagsMap[exportedTag.Key]; !exists {
				uniqueTagsMap[exportedTag.Key] = exportedTag
			}
		}
	}

	exportedTags := make([]tagemonv1alpha1.ExportedTag, 0, len(uniqueTagsMap))
	for _, tag := range uniqueTagsMap {
		exportedTags = append(exportedTags, tag)
	}

	return exportedTags
}

func (h *Handler) isResourceRelevant(resource interface{}, allowedAccountIDs map[string]bool, searchTags []tagemonv1alpha1.TagemonTag) bool {

	type CloudResource interface {
		ID() string
		Tags() map[string]string
	}

	r, ok := resource.(CloudResource)
	if !ok {
		return false
	}

	accountID := h.extractAccountID(r.ID())
	if len(allowedAccountIDs) > 0 && !allowedAccountIDs[accountID] {
		return false
	}

	if len(searchTags) > 0 {
		resourceTags := r.Tags()

		for _, searchTag := range searchTags {
			if tagValue, exists := resourceTags[searchTag.Key]; !exists || tagValue != searchTag.Value {
				return false
			}
		}
	}

	return true
}

func (h *Handler) getOrCreateMetric(tagKey string, exportedTags []tagemonv1alpha1.ExportedTag) *prometheus.GaugeVec {
	logger := log.FromContext(context.Background())
	metricName := h.tagToMetricName(tagKey)

	metricKey := metricName
	if len(exportedTags) > 0 {
		exportedKeys := make([]string, 0, len(exportedTags))
		for _, tag := range exportedTags {
			exportedKeys = append(exportedKeys, tag.Key)
		}
		metricKey = fmt.Sprintf("%s_%s", metricName, strings.Join(exportedKeys, "_"))
	}

	if gauge, exists := h.metricsGauges[metricKey]; exists {
		return gauge
	}

	labelNames := []string{"tag_Name", "account_id", "type"}
	for _, exportedTag := range exportedTags {
		labelNames = append(labelNames, "tag_"+exportedTag.Key)
	}

	gauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: metricName,
			Help: fmt.Sprintf("Tagemon threshold tag value for %s", tagKey),
		},
		labelNames,
	)

	if err := metrics.Registry.Register(gauge); err != nil {
		logger.Error(err, "Failed to register metric",
			"metricName", metricName,
			"metricKey", metricKey,
			"labelNames", labelNames,
			"tagKey", tagKey)
		return nil
	}

	h.metricsGauges[metricKey] = gauge

	return gauge
}

func (h *Handler) getOrCreateNonCompliantMetric() *prometheus.GaugeVec {
	metricName := "tagemon_resources_non_compliant_count"

	if gauge, exists := h.nonCompliantGauges[metricName]; exists {
		return gauge
	}

	gauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: metricName,
			Help: "Current count of non-compliant resources by type and account",
		},
		[]string{"resource_type", "account_id"},
	)

	metrics.Registry.MustRegister(gauge)
	h.nonCompliantGauges[metricName] = gauge

	return gauge
}

func (h *Handler) tagToMetricName(tagKey string) string {
	metricName := strings.ToLower(tagKey)
	reg := regexp.MustCompile(`[^a-z0-9_]`)
	metricName = reg.ReplaceAllString(metricName, "_")
	reg = regexp.MustCompile(`_+`)
	metricName = reg.ReplaceAllString(metricName, "_")
	metricName = strings.Trim(metricName, "_")

	return "tagemon_" + metricName
}

// =============================================================================
// SCHEDULER
// =============================================================================

type Scheduler struct {
	mgr      ctrl.Manager
	handler  *Handler
	config   *confighandler.Config
	interval time.Duration
}

func (s *Scheduler) Start(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("tagshandler-scheduler")

	logger.Info("Waiting for cache to sync before running initial compliance check")
	if !s.mgr.GetCache().WaitForCacheSync(ctx) {
		return fmt.Errorf("failed to wait for caches to sync")
	}

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

func (h *Handler) extractResourceName(resourceARN string, tags map[string]string) string {
	if name, exists := tags["Name"]; exists && name != "" {
		return name
	}

	for _, nameKey := range []string{"name", "resource-name", "ResourceName"} {
		if name, exists := tags[nameKey]; exists && name != "" {
			return name
		}
	}

	parts := strings.Split(resourceARN, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}

	return resourceARN
}

func (h *Handler) extractAccountID(resourceARN string) string {
	// ARN format: arn:partition:service:region:account-id:resource-type/resource-id
	parts := strings.Split(resourceARN, ":")
	if len(parts) >= 5 {
		return parts[4]
	}
	return "unknown"
}

func (h *Handler) processResults(ctx context.Context, results []patrol.Result, thresholdTagsMap map[string]map[string]tagemonv1alpha1.ThresholdTagType, allowedAccountIDs map[string]bool, searchTags []tagemonv1alpha1.TagemonTag, exportedTags []tagemonv1alpha1.ExportedTag) *ComplianceReport {
	logger := log.FromContext(ctx)

	h.resetAllThresholdMetrics()

	totalResources := 0
	compliantResources := 0
	violatingResources := 0

	nonCompliantCounts := make(map[string]map[string]int)
	allResourceTypes := make(map[string]map[string]bool)

	for _, patrolResult := range results {
		if patrolResult.Error != nil {
			logger.Error(patrolResult.Error, "Error processing patrol result", "service", patrolResult.Definition.Service)
			continue
		}

		serviceType := patrolResult.Definition.Service
		thresholdTags := thresholdTagsMap[serviceType]

		if len(patrolResult.Resources) == 0 {
			resourceType := "unknown"
			if patrolResult.Definition.ResourceType != "" {
				resourceType = patrolResult.Definition.ResourceType
			}

			logger.Info("Empty response for resource type",
				"service", serviceType,
				"resourceType", resourceType,
				"message", "Empty response for type: "+resourceType+", this can indicate a wrong type has been provided")
		}

		for _, resource := range patrolResult.Resources {
			if !h.isResourceRelevant(resource, allowedAccountIDs, searchTags) {
				// TODO: Uncomment this when we have a way to debug irrelevant resources
				// logger.V(1).Info("Skipping irrelevant resource",
				// 	"resource", resource.ID(),
				// 	"reason", "does not match monitored accounts or searchTags criteria")
				continue
			}

			totalResources++
			tagName := h.extractResourceName(resource.ID(), resource.Tags())
			accountID := h.extractAccountID(resource.ID())
			resourceType := fmt.Sprintf("%s/%s", strings.ToLower(resource.Service()), strings.ToLower(resource.Type()))

			if allResourceTypes[resourceType] == nil {
				allResourceTypes[resourceType] = make(map[string]bool)
			}
			allResourceTypes[resourceType][accountID] = true

			if resource.IsCompliant() {
				compliantResources++
				h.createMetricsForResource(resource, thresholdTags, tagName, accountID, resourceType, exportedTags)
			} else {
				violatingResources++
				h.handleNonCompliantResource(resource, tagName, accountID, resourceType)

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

func (h *Handler) createMetricsForResource(resource interface{}, thresholdTags map[string]tagemonv1alpha1.ThresholdTagType, tagName, accountID, resourceType string, exportedTags []tagemonv1alpha1.ExportedTag) {
	type CloudResource interface {
		ID() string
		Tags() map[string]string
	}

	r, ok := resource.(CloudResource)
	if !ok {
		return
	}

	tags := r.Tags()

	exportedTagValues := make([]string, 0, len(exportedTags))
	for _, exportedTag := range exportedTags {
		tagValue := ""
		if val, exists := tags[exportedTag.Key]; exists {
			tagValue = val
		}
		exportedTagValues = append(exportedTagValues, tagValue)
	}

	for tagKey, tagType := range thresholdTags {
		if tagValue, exists := tags[tagKey]; exists && tagValue != "" {
			gauge := h.getOrCreateMetric(tagKey, exportedTags)

			if gauge == nil {
				continue
			}

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
				labelValues := []string{tagName, accountID, resourceType}
				labelValues = append(labelValues, exportedTagValues...)
				gauge.WithLabelValues(labelValues...).Set(value)
			}
		}
	}
}

func (h *Handler) handleNonCompliantResource(resource interface{}, tagName, accountID, resourceType string) {
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
		"resource", tagName,
		"type", resourceType,
		"arn", resourceARN,
		"account", accountID,
		"violations", violations)
}

func (h *Handler) resetAllThresholdMetrics() {
	for _, gauge := range h.metricsGauges {
		gauge.Reset()
	}
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
