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
	"github.com/stretchr/testify/assert"
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
