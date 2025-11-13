package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AWSRegion represents a valid AWS region
// +kubebuilder:validation:Enum=us-east-1;us-east-2;us-west-1;us-west-2;af-south-1;ap-east-1;ap-south-1;ap-south-2;ap-northeast-1;ap-northeast-2;ap-northeast-3;ap-southeast-1;ap-southeast-2;ap-southeast-3;ap-southeast-4;ca-central-1;eu-central-1;eu-central-2;eu-north-1;eu-south-1;eu-south-2;eu-west-1;eu-west-2;eu-west-3;me-central-1;me-south-1;sa-east-1
type AWSRegion string

// AWSRole represents an AWS IAM role to assume
type AWSRole struct {
	// RoleArn is the ARN of the AWS IAM role to assume
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^arn:aws:iam::\d{12}:role\/[A-Za-z0-9+=,.@_\-/]+$`
	RoleArn string `json:"roleArn"`

	// ExternalId is the external ID to use when assuming the role (optional)
	// +kubebuilder:validation:Optional
	ExternalId string `json:"externalId,omitempty"`
}

// Statistics represents a valid AWS metrics statistics
// +kubebuilder:validation:Enum=Sum;Average;Maximum;Minimum;SampleCount
type Statistics string

// PodResources captures resource requests and limits for the YACE pod's container.
// +kubebuilder:validation:Optional
type PodResources struct {
	// Requests describes the minimum amount of compute resources required.
	// Keys are valid resource names (e.g. cpu, memory). Values follow Kubernetes quantity format.
	// +kubebuilder:validation:Optional
	Requests corev1.ResourceList `json:"requests,omitempty"`

	// Limits describes the maximum amount of compute resources allowed.
	// Keys are valid resource names (e.g. cpu, memory). Values follow Kubernetes quantity format.
	// +kubebuilder:validation:Optional
	Limits corev1.ResourceList `json:"limits,omitempty"`
}

// TagemonSpec defines the desired state of Tagemon.
type TagemonSpec struct {
	// Type is the AWS service type (e.g., AWS/RDS, AWS/Backup, AWS/S3)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^AWS\/[A-Za-z0-9]+$`
	Type string `json:"type"`

	// ResourceExplorerService optionally specifies the AWS Resource Explorer service type
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Pattern=`^[A-Za-z0-9-]+$`
	ResourceExplorerService *string `json:"resourceExplorerService,omitempty"`

	// Regions is the list of AWS regions to monitor
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Regions []AWSRegion `json:"regions"`

	// AWS roles to assume
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Roles []AWSRole `json:"awsRoles"`

	// Statistics are the default statistics for all metrics, can be overridden in the metric configuration
	// +kubebuilder:validation:Required
	Statistics []Statistics `json:"statistics,omitempty"`

	// Period is the default period in seconds for CloudWatch metrics, can be overridden in the metric configuration
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	Period *int32 `json:"period,omitempty"`

	// ScrapingInterval is the global scraping interval in seconds for all metrics
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=60
	ScrapingInterval *int32 `json:"scrapingInterval,omitempty"`

	// NilToZero is the default setting to convert nil values to zero, can be overridden in the metric configuration
	// +kubebuilder:default=true
	NilToZero *bool `json:"nilToZero,omitempty"`

	// AddCloudwatchTimestamp is the default setting to add CloudWatch timestamp, can be overridden in the metric configuration
	// +kubebuilder:validation:Optional
	AddCloudwatchTimestamp *bool `json:"addCloudwatchTimestamp,omitempty"`

	// Metrics is the list of metrics to monitor
	// +kubebuilder:validation:Required
	Metrics []TagemonMetric `json:"metrics,omitempty"`

	// SearchTags are optional tags to filter resources
	// +kubebuilder:validation:Optional
	SearchTags []TagemonTag `json:"searchTags,omitempty"`

	// +kubebuilder:validation:Optional
	ExportedTagsOnMetrics []ExportedTag `json:"exportedTagsOnMetrics,omitempty"`

	// DimensionNameRequirements filters metrics to only those with the specified dimensions
	// +kubebuilder:validation:Optional
	DimensionNameRequirements []string `json:"dimensionNameRequirements,omitempty"`

	// PodResources defines resource requirements for the YACE pods
	// +kubebuilder:validation:Optional
	PodResources *PodResources `json:"podResources,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=200
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	NamePrefix string `json:"namePrefix,omitempty"`
}

// TagemonMetric defines a CloudWatch metric configuration
type TagemonMetric struct {
	// Name is the CloudWatch metric name
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Statistics For Metric
	// +kubebuilder:validation:Optional
	Statistics []Statistics `json:"statistics,omitempty"`

	// Period For Metric
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=2
	Period *int32 `json:"period,omitempty"`

	// NilToZero For Metric
	// +kubebuilder:validation:Optional
	NilToZero *bool `json:"nilToZero,omitempty"`

	// AddCloudwatchTimestamp For Metric
	// +kubebuilder:validation:Optional
	AddCloudwatchTimestamp *bool `json:"addCloudwatchTimestamp,omitempty"`

	// ThresholdTags is the list of threshold tag configurations for this metric
	// +kubebuilder:validation:Optional
	ThresholdTags []ThresholdTag `json:"thresholdTags,omitempty"`
}

// TagemonTag defines a tag key-value pair for resource filtering
type TagemonTag struct {
	// Key is the tag key
	// +kubebuilder:validation:Required
	Key string `json:"key"`

	// Value is the tag value
	// +kubebuilder:validation:Required
	Value string `json:"value"`
}

// ExportedTag defines a tag to be exported on metrics
type ExportedTag struct {
	// Key is the tag key to export
	// +kubebuilder:validation:Required
	Key string `json:"key"`

	// Required indicates whether this tag is mandatory for compliance
	// +kubebuilder:default=false
	// +kubebuilder:validation:Optional
	Required *bool `json:"required,omitempty"`
}

// ThresholdTagType represents the type of threshold tag value
// +kubebuilder:validation:Enum=int;bool;percentage
type ThresholdTagType string

const (
	ThresholdTagTypeInt        ThresholdTagType = "int"
	ThresholdTagTypeBool       ThresholdTagType = "bool"
	ThresholdTagTypePercentage ThresholdTagType = "percentage"
)

// ThresholdTag defines a threshold tag configuration
type ThresholdTag struct {
	// Type is the type of the threshold tag value
	// +kubebuilder:validation:Required
	Type ThresholdTagType `json:"type"`

	// Key is the tag key to monitor
	// +kubebuilder:validation:Required
	Key string `json:"key"`

	// ResourceType is the AWS resource type to apply this threshold tag to
	// +kubebuilder:validation:Required
	ResourceType string `json:"resourceType"`

	// Required indicates whether this threshold tag is mandatory for compliance
	// +kubebuilder:default=true
	// +kubebuilder:validation:Optional
	Required *bool `json:"required,omitempty"`
}

// TagemonStatus defines the observed state of Tagemon.
type TagemonStatus struct {
	// ObservedGeneration reflects the generation of the most recently observed Tagemon.
	// +optional
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
	DeploymentID       string `json:"deploymentID,omitempty"`

	// ConfigMapName is the name of the ConfigMap created for this Tagemon
	// +optional
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	ConfigMapName string `json:"configMapName,omitempty"`

	// DeploymentName is the name of the Deployment created for this Tagemon
	// +optional
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	DeploymentName string `json:"deploymentName,omitempty"`

	// ServiceAccountName is the name of the ServiceAccount created for this Tagemon
	// +optional
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type",description="AWS service type"
// +kubebuilder:printcolumn:name="Regions",type="string",JSONPath=".spec.regions",description="Monitored regions"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Tagemon is the Schema for the tagemons API.
type Tagemon struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TagemonSpec   `json:"spec,omitempty"`
	Status TagemonStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TagemonList contains a list of Tagemon.
type TagemonList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Tagemon `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Tagemon{}, &TagemonList{})
}
