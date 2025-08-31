package thresholdshandler

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/resourceexplorer2"
	"github.com/aws/aws-sdk-go-v2/service/resourceexplorer2/types"
	"github.com/aws/smithy-go/document"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	tagemonv1alpha1 "github.com/next-insurance/tagemon-dev/api/v1alpha1"
)

const (
	unableToParse = "<unable to parse>"
)

type ThresholdMonitor struct {
	client           client.Client
	resourceExplorer *resourceexplorer2.Client
}

type ThresholdTagsByType struct {
	TagemonType string
	Tags        []string
}

func NewThresholdMonitor(kubeClient client.Client) (*ThresholdMonitor, error) {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	resourceExplorer := resourceexplorer2.NewFromConfig(cfg)

	return &ThresholdMonitor{
		client:           kubeClient,
		resourceExplorer: resourceExplorer,
	}, nil
}

func (tm *ThresholdMonitor) Start(ctx context.Context) {
	logger := log.FromContext(ctx)
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	logger.Info("Starting threshold tags monitor - will run every minute")

	// Run immediately on start
	tm.runMonitoring(ctx)

	for {
		select {
		case <-ctx.Done():
			logger.Info("Threshold monitor context cancelled, stopping...")
			return
		case <-ticker.C:
			tm.runMonitoring(ctx)
		}
	}
}

func (tm *ThresholdMonitor) runMonitoring(ctx context.Context) {
	logger := log.FromContext(ctx)
	logger.Info("Running threshold tags monitoring")

	tagemons, err := tm.getAllTagemonCRs(ctx)
	if err != nil {
		logger.Error(err, "Error retrieving Tagemon CRs")
		return
	}

	if len(tagemons) == 0 {
		logger.Info("No Tagemon CRs found")
		return
	}

	logger.Info("Found Tagemon CRs", "count", len(tagemons))

	tagsByType := tm.groupThresholdTagsByType(tagemons)

	if len(tagsByType) == 0 {
		logger.Info("No threshold tags found in any Tagemon CRs")
		return
	}

	logger.Info("Found threshold tags for resource types", "typeCount", len(tagsByType))

	for _, tagGroup := range tagsByType {
		tm.queryResourceExplorerByType(ctx, tagGroup)
	}

	logger.Info("Threshold tags monitoring complete")
}

func (tm *ThresholdMonitor) getAllTagemonCRs(ctx context.Context) ([]tagemonv1alpha1.Tagemon, error) {
	var tagemonList tagemonv1alpha1.TagemonList
	if err := tm.client.List(ctx, &tagemonList); err != nil {
		return nil, fmt.Errorf("failed to list Tagemon CRs: %w", err)
	}

	return tagemonList.Items, nil
}

func (tm *ThresholdMonitor) groupThresholdTagsByType(tagemons []tagemonv1alpha1.Tagemon) []ThresholdTagsByType {
	typeToTags := make(map[string]map[string]bool)

	for _, tagemon := range tagemons {
		if len(tagemon.Spec.ThresholdTags) == 0 {
			continue
		}

		if typeToTags[tagemon.Spec.Type] == nil {
			typeToTags[tagemon.Spec.Type] = make(map[string]bool)
		}

		for _, tag := range tagemon.Spec.ThresholdTags {
			if tag != "" {
				typeToTags[tagemon.Spec.Type][tag] = true
			}
		}
	}

	result := make([]ThresholdTagsByType, 0, len(typeToTags))
	for tagemonType, tagSet := range typeToTags {
		var tags []string
		for tag := range tagSet {
			tags = append(tags, tag)
		}

		result = append(result, ThresholdTagsByType{
			TagemonType: tagemonType,
			Tags:        tags,
		})
	}

	return result
}

func (tm *ThresholdMonitor) extractServiceFromTagemonType(tagemonType string) string {
	if strings.HasPrefix(tagemonType, "AWS/") {
		serviceName := strings.ToLower(strings.TrimPrefix(tagemonType, "AWS/"))

		switch serviceName {
		case "applicationelb", "networkelb":
			return "elasticloadbalancing"
		case "elb":
			return "elasticloadbalancing"
		default:
			return serviceName
		}
	}

	return strings.ToLower(tagemonType)
}

func (tm *ThresholdMonitor) queryResourceExplorerByType(ctx context.Context, tagGroup ThresholdTagsByType) {
	logger := log.FromContext(ctx)
	serviceName := tm.extractServiceFromTagemonType(tagGroup.TagemonType)
	logger.Info("Processing threshold tags for resource type", "tagCount", len(tagGroup.Tags), "tagemonType", tagGroup.TagemonType, "serviceName", serviceName)

	for _, tagName := range tagGroup.Tags {
		tm.queryResourceExplorerForTag(ctx, tagName, tagGroup.TagemonType, serviceName)
	}
}

func (tm *ThresholdMonitor) queryResourceExplorerForTag(ctx context.Context, tagName, tagemonType, serviceName string) {
	logger := log.FromContext(ctx)
	logger.Info("Querying Resource Explorer for tag", "tag", tagName, "tagemonType", tagemonType, "serviceName", serviceName)

	query := fmt.Sprintf("tag.key:%s service:%s", tagName, serviceName)

	input := &resourceexplorer2.SearchInput{
		QueryString: aws.String(query),
		MaxResults:  aws.Int32(50),
	}

	result, err := tm.resourceExplorer.Search(ctx, input)
	if err != nil {
		logger.Error(err, "Error querying Resource Explorer for tag", "tag", tagName, "tagemonType", tagemonType)
		return
	}

	if len(result.Resources) == 0 {
		logger.Info("No resources found with tag", "tagemonType", tagemonType, "tag", tagName)
		return
	}

	logger.Info("Found resources with tag", "resourceCount", len(result.Resources), "tagemonType", tagemonType, "tag", tagName)

	for _, resource := range result.Resources {
		resourceARN := aws.ToString(resource.Arn)
		resourceType := aws.ToString(resource.ResourceType)

		logger.Info("Found resource", "arn", resourceARN, "resourceType", resourceType)

		tagValue := tm.extractTagValue(resource.Properties, tagName)
		logger.Info("Tag value on resource", "tag", tagName, "value", tagValue, "arn", resourceARN, "resourceType", resourceType)
	}
}

func (tm *ThresholdMonitor) extractTagValue(properties []types.ResourceProperty, tagName string) string {
	for _, prop := range properties {
		propName := aws.ToString(prop.Name)

		if propName == "Tags" || propName == "tags" || propName == "TagSet" {
			if prop.Data != nil {
				tagValue := tm.parseTagFromData(prop.Data, tagName)
				if tagValue != unableToParse {
					return tagValue
				}
			}
		}

		if propName == tagName {
			if prop.Data != nil {
				return fmt.Sprintf("%v", prop.Data)
			}
		}
	}

	return "<value not available>"
}

func (tm *ThresholdMonitor) parseTagFromData(data interface{}, tagName string) string {
	if _, isDocument := data.(document.Unmarshaler); isDocument {
		dataStr := fmt.Sprintf("%v", data)
		if strings.Contains(dataStr, fmt.Sprintf("Key:%s", tagName)) {
			return tm.extractTagFromDocumentString(dataStr, tagName)
		}
		return "<tag not found in document>"
	}

	actualData := data

	switch v := actualData.(type) {
	case map[string]interface{}:
		if tagValue, exists := v[tagName]; exists {
			return fmt.Sprintf("%v", tagValue)
		}

		for key, value := range v {
			if key == tagName {
				return fmt.Sprintf("%v", value)
			}
			if nestedMap, ok := value.(map[string]interface{}); ok {
				if tagValue, exists := nestedMap[tagName]; exists {
					return fmt.Sprintf("%v", tagValue)
				}
			}
		}

	case []interface{}:
		for _, item := range v {
			if tagMap, ok := item.(map[string]interface{}); ok {
				if key, keyExists := tagMap["Key"]; keyExists && fmt.Sprintf("%v", key) == tagName {
					if value, valueExists := tagMap["Value"]; valueExists {
						return fmt.Sprintf("%v", value)
					}
				}
			}
		}

	case string:
		if strings.Contains(v, tagName) {
			return "<found in JSON - parsing needed>"
		}
	}

	return unableToParse
}

func (tm *ThresholdMonitor) extractTagFromDocumentString(dataStr, tagName string) string {
	pattern := fmt.Sprintf(`map\[Key:%s Value:([^\]]+)\]`, regexp.QuoteMeta(tagName))
	re := regexp.MustCompile(pattern)

	matches := re.FindStringSubmatch(dataStr)
	if len(matches) > 1 {
		return matches[1]
	}

	return "<extraction failed>"
}
