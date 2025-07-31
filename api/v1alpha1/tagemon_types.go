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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AWSRegion represents a valid AWS region
// +kubebuilder:validation:Enum=us-east-1;us-east-2;us-west-1;us-west-2;af-south-1;ap-east-1;ap-south-1;ap-south-2;ap-northeast-1;ap-northeast-2;ap-northeast-3;ap-southeast-1;ap-southeast-2;ap-southeast-3;ap-southeast-4;ca-central-1;eu-central-1;eu-central-2;eu-north-1;eu-south-1;eu-south-2;eu-west-1;eu-west-2;eu-west-3;me-central-1;me-south-1;sa-east-1
type AWSRegion string

// Statistics represents a valid AWS metrics statistics
// +kubebuilder:validation:Enum=Sum;Average;Maximum;Minimum;SampleCount
type Statistics string

// TagemonSpec defines the desired state of Tagemon.
type TagemonSpec struct {
	// Type is the AWS service type (e.g., AWS/RDS, AWS/Backup, AWS/S3)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^AWS\/[A-Z]+$`
	Type string `json:"type"`

	// Regions is the list of AWS regions to monitor
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Regions []AWSRegion `json:"regions"`

	// Statistics are the default statistics for all metrics, can be overridden in the metric configuration
	// +kubebuilder:validation:Required
	Statistics []Statistics `json:"statistics,omitempty"`

	// Period is the default period in seconds for CloudWatch metrics, can be overridden in the metric configuration
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	Period *int32 `json:"period,omitempty"`

	// Length is the default length in seconds for metric data retrieval, can be overridden in the metric configuration
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	Length *int32 `json:"length,omitempty"`

	// Delay is the default delay in seconds before retrieving metrics, can be overridden in the metric configuration
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	Delay *int32 `json:"delay,omitempty"`

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

	// ThresholdTags is the list of threshold tag names
	// +kubebuilder:validation:Optional
	ThresholdTags []string `json:"thresholdTags,omitempty"`
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

	// Length For Metric
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	Length *int32 `json:"length,omitempty"`

	// Delay For Metric
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	Delay *int32 `json:"delay,omitempty"`

	// NilToZero For Metric
	// +kubebuilder:validation:Optional
	NilToZero *bool `json:"nilToZero,omitempty"`

	// AddCloudwatchTimestamp For Metric
	// +kubebuilder:validation:Optional
	AddCloudwatchTimestamp *bool `json:"addCloudwatchTimestamp,omitempty"`
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

// TagemonStatus defines the observed state of Tagemon.
type TagemonStatus struct {
	// ObservedGeneration reflects the generation of the most recently observed Tagemon.
	// +optional
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
	DeploymentID       string `json:"deploymentID,omitempty"`
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
