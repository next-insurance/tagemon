# Tagemon

[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)
[![Tests](https://github.com/next-insurance/tagemon/actions/workflows/tests.yml/badge.svg?branch=main)](https://github.com/next-insurance/tagemon/actions/workflows/tests.yml)
[![Helm Release](https://github.com/next-insurance/tagemon/actions/workflows/helm-release.yml/badge.svg?branch=main)](https://github.com/next-insurance/tagemon/actions/workflows/helm-release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/next-insurance/tagemon)](https://goreportcard.com/report/github.com/next-insurance/tagemon)

A Kubernetes operator for monitoring AWS CloudWatch metrics with tag-based compliance validation.

## 🚀 Why Tagemon?

Monitoring large-scale AWS environments often leads to **Alert Fatigue** or **Blind Spots** because static configurations can't keep up with dynamic infrastructure. Tagemon transforms your monitoring from static configuration files into a tag-driven ecosystem.

### The Problem

- **Manual Overhead**: Manually updating exporter configuration every time a new RDS instance or Auto Scaling Group is launched
- **Inconsistent Alerting**: Maintaining separate alert rules for each resource instead of a single alert rule that applies to all resources with tag-based dynamic thresholds
- **Monitoring Gaps**: Resources launched but not added to monitoring configuration become invisible, creating blind spots until someone manually discovers and configures them

### The Solution

- **Infrastructure as Code via Tags**: Stop editing YAML to monitor new resources. Simply tag an AWS resource, and Tagemon automatically discovers it and starts exporting its metrics to Prometheus
- **Decentralized Thresholds**: Empower developers to define their own alerting thresholds directly on AWS resource tags. Tagemon exports these as metrics, allowing for a single, universal Prometheus alert rule that respects per-resource limits
- **Enforced Compliance**: Automatically identify "Non-Compliant" resources that lack mandatory tags and view these gaps as Prometheus metrics to drive better cloud governance

### Key Benefits

- **Reduce TTR (Time to Response)**: Monitoring is active the second a resource is tagged
- **FinOps & Governance**: Ensure 100% of your infrastructure is tagged and accounted for by tracking the `tagemon_resources_non_compliant_count` metric
- **Centralized Management**: Single point of control for all CloudWatch monitoring across your organization, eliminating configuration drift and ensuring uniform metric collection standards

## Features

- 🎯 **Tag-Based Monitoring**: Monitor AWS resources based on tags
- 📊 **CloudWatch Integration**: Export CloudWatch metrics via YACE (Yet Another CloudWatch Exporter)
- ✅ **Compliance Checking**: Validate resources against tag policies
- 🔄 **Dynamic Configuration**: Automatic deployment updates based on CRD changes
- 📈 **Prometheus Compatible**: Native Prometheus metrics exposition

## Quick Start

### Prerequisites

- kubectl configured
- Helm 3.x (for Helm installation)
- AWS credentials with appropriate permissions
- AWS Resource Explorer configured with a view ARN

### Installation

#### Helm Installation (Recommended)

Create a `values.yaml` file:

```yaml
controller:
  serviceAccountName: tagemon-controller-manager
  config:
    yaceServiceAccountName: tagemon-yace
    tagsHandler:
      viewArn: "arn:aws:resource-explorer-2:us-west-2:ACCOUNT:view/NAME/ID"
      region: "us-west-2"
      interval: "60m"
```

Install the chart:

```bash
helm install tagemon oci://docker.io/nextinsurancedevops/tagemon \
  --namespace tagemon \
  --create-namespace \
  -f values.yaml
```

📖 **For all available configuration options, see [chart/README.md](chart/README.md)**

#### Manual Installation

```bash
# Install CRDs
kubectl apply -f config/crd/bases/

# Deploy the operator
kubectl apply -f config/default/
```

## Tagemon CRD Specification

### Spec Fields Reference

#### Required Fields

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `type` | string | AWS service namespace (format: `AWS/SERVICE`) | `AWS/EC2`, `AWS/RDS`, `AWS/ELB` |
| `regions` | []string | List of AWS regions to monitor | `[us-east-1, us-west-2]` |
| `awsRoles` | []object | IAM roles for CloudWatch access | See [AWS Roles](#aws-roles) |
| `statistics` | []string | Default statistics for metrics | `[Average, Maximum, Sum]` |
| `period` | int32 | Default CloudWatch period in seconds (min: 1) | `300` (5 minutes) |
| `metrics` | []object | Metrics to collect | See [Metrics](#metrics) |

#### Optional Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `scrapingInterval` | int32 | - | Global scraping interval in seconds (min: 60) |
| `nilToZero` | bool | `true` | Convert nil metric values to zero |
| `addCloudwatchTimestamp` | bool | `false` | Include CloudWatch timestamp in metrics |
| `searchTags` | []object | - | Filter resources by tags |
| `exportedTagsOnMetrics` | []object | - | Tags to export as Prometheus labels |
| `dimensionNameRequirements` | []string | - | Required CloudWatch dimension names for metrics |
| `podResources` | object | - | Resource requests/limits for YACE pods |
| `namePrefix` | string | - | Prefix for generated resource names (max 200 chars) |
| `resourceExplorerService` | string | - | Override AWS Resource Explorer service type (optional) |

### Complete Example

This example demonstrates all features of Tagemon, including required fields and optional configurations:

```yaml
apiVersion: tagemon.io/v1alpha1
kind: Tagemon
metadata:
  name: tagemon-rds-monitoring
  namespace: tagemon
spec:
  # Required fields
  type: AWS/RDS
  regions:
    - us-east-1
    - us-west-2
  awsRoles:
    - roleArn: arn:aws:iam::123456789012:role/TagemonRole
  statistics: [Maximum]
  period: 60
  metrics:
    - name: CPUUtilization  # name is required
      statistics: [Average]  # optional: overrides spec.statistics
      period: 60            # optional: overrides spec.period
      thresholdTags:         # optional: tag-based compliance
        - type: int
          key: max_cpu_utilization_percent_threshold
          resourceType: db
          required: true    # optional: defaults to true
    
    - name: DBLoad
      statistics: [Maximum]  # optional
      period: 60            # optional
      thresholdTags:         # optional
        - type: int
          key: max_db_load_threshold
          resourceType: db
          required: true    # optional
    
    - name: FreeStorageSpace
      statistics: [Minimum]  # optional
      period: 60            # optional
      thresholdTags:         # optional
        - type: int
          key: allocated_storage_threshold
          resourceType: db
          required: false   # optional
        - type: int
          key: low_free_storage_percent_threshold
          resourceType: db
          required: false   # optional
        - type: int
          key: lowest_free_storage_percent_threshold
          resourceType: db
          required: false   # optional
        - type: int
          key: storage_drop_mb_threshold
          resourceType: db
          required: false   # optional
    
    - name: ReplicaLag
      statistics: [Minimum, Maximum]  # optional
      period: 60                      # optional
      thresholdTags:                   # optional
        - type: int
          key: high_replica_lag_threshold
          resourceType: db
          required: false             # optional
  
  # Optional fields
  scrapingInterval: 240
  nilToZero: true
  
  # Optional: Filter resources by tags - only monitor resources with these tags
  searchTags:
    - key: monitor
      value: "true"
  
  # Optional: Dimension requirements for CloudWatch metrics
  dimensionNameRequirements:
    - DBInstanceIdentifier
  
  # Optional: Export tags as Prometheus labels
  exportedTagsOnMetrics:
    - key: Environment
      required: true
    - key: DatabaseName
      required: false
  
  # Optional: Resource limits for YACE pods
  podResources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 500m
      memory: 256Mi
```

### AWS Roles

Configure IAM roles for CloudWatch access:

```yaml
awsRoles:
  - roleArn: arn:aws:iam::123456789012:role/TagemonRole
```

**Fields:**
- `roleArn` (required): IAM role ARN (pattern: `arn:aws:iam::ACCOUNT:role/NAME`)

### Metrics

Define which CloudWatch metrics to collect. Each metric can override global settings and optionally define threshold tags for alerting.

**Fields:**
- `name` (required): CloudWatch metric name
- `statistics` (optional): Override default statistics (`Sum`, `Average`, `Maximum`, `Minimum`, `SampleCount`)
- `period` (optional): Override default period (min: 2 seconds)
- `nilToZero` (optional): Override default nil-to-zero behavior
- `addCloudwatchTimestamp` (optional): Override timestamp behavior
- `thresholdTags` (optional): Define tag-based thresholds for compliance validation (see [Threshold Tags](#threshold-tags) below)

See the [complete example](#complete-example) above for usage examples.

### Threshold Tags

Validate resources against tag-based thresholds:

```yaml
thresholdTags:
  - type: percentage        # int, bool, or percentage
    key: cpu-threshold      # Tag key on AWS resource
    resourceType: instance  # AWS resource type (e.g., instance, database)
    required: true          # Whether tag is mandatory (default: true)
```

**Threshold Types:**
- `percentage`: Value is a percentage (0-100)
- `int`: Value is an integer
- `bool`: Value is a boolean (true/false)

**Example Use Case:**
Tag an EC2 instance with `cpu-threshold: 80`, and Tagemon will validate that the instance has this tag and can use it for alerting thresholds.

**Alerting with Threshold Tags:**

Tagemon exposes threshold values as Prometheus metrics that can be used in alerting rules. The metric name follows the pattern `tagemon_{threshold_tag_key}` with labels `tag_Name` and `account_id`.

Example Prometheus alert rule that applies to all RDS instances using threshold tags:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: rds-high-cpu-alert
  namespace: tagemon
spec:
  groups:
    - name: aws-rds
      rules:
        - alert: RDSHighCPUUtilization
          annotations:
            description: DB {{ $labels.tag_Name }} has a high CPU utilization
            summary: 'DB {{ $labels.tag_Name }} has a high CPU Utilization, Utilization: {{ $value | printf "%.0f%%" }}'
          expr: aws_rds_cpuutilization_average > on(tag_Name, account_id) tagemon_max_cpu_utilization_percent_threshold
          for: 10m
          labels:
            severity: warning
```

This alert rule automatically applies to all RDS instances monitored by Tagemon. Each RDS instance can have its own threshold value defined via the `max_cpu_utilization_percent_threshold` tag, allowing per-resource alerting thresholds. In this example:
- `aws_rds_cpuutilization_average` is the CloudWatch metric exported by YACE for all RDS instances
- `tagemon_max_cpu_utilization_percent_threshold` is the threshold metric created by Tagemon from the tag on each RDS instance
- The `on(tag_Name, account_id)` clause ensures the comparison is done per-resource, matching each RDS instance's CPU utilization against its own threshold value
- The alert fires for any RDS instance when its CPU utilization exceeds the threshold value defined in its tag

### Search Tags

Filter which AWS resources to monitor based on tags:

```yaml
searchTags:
  - key: Environment
    value: production
  - key: Team
    value: platform
```

Only resources matching **all** specified tags will be monitored.

### Exported Tags on Metrics

Export AWS resource tags as Prometheus labels:

```yaml
exportedTagsOnMetrics:
  - key: Environment
    required: false    # If true, resources without this tag will be filtered out
  - key: Application
    required: true
```

**Note:** This allows you to query metrics by tag values in Prometheus (e.g., `{Environment="production"}`).

### Pod Resources

Configure resource requests and limits for YACE pods. See the [complete example](#complete-example) above for usage.

```yaml
podResources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 500m
    memory: 256Mi
```

## AWS Prerequisites

### Required AWS Setup

1. **AWS Resource Explorer**: Enable and create a view for tag-based compliance validation
   - Example ARN: `arn:aws:resource-explorer-2:us-west-2:123456789012:view/MainView/abc123`
   - [Setup Guide](https://docs.aws.amazon.com/resource-explorer/latest/userguide/getting-started.html)

2. **IAM Permissions**: Configure IAM roles for both service accounts using [IRSA (IAM Roles for Service Accounts)](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html)

### Controller Service Account (`tagemon-controller`)

The controller service account needs permissions for tag compliance validation:

```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": [
      "resource-explorer-2:Search"
    ],
    "Resource": "*"
  }]
}
```

### YACE Service Account (`tagemon-yace`)

The YACE service account needs permissions to assume YACE roles and collect CloudWatch metrics. See [YACE Authentication documentation](https://github.com/prometheus-community/yet-another-cloudwatch-exporter?tab=readme-ov-file#authentication) for details.

**Minimum required permissions:**
```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": [
      "tag:GetResources",
      "cloudwatch:GetMetricData",
      "cloudwatch:GetMetricStatistics",
      "cloudwatch:ListMetrics",
      "sts:AssumeRole"
    ],
    "Resource": "*"
  }]
}
```

**Note:** Additional permissions may be required depending on the AWS services you're monitoring (e.g., `apigateway:GET` for API Gateway, `autoscaling:DescribeAutoScalingGroups` for Auto Scaling). See the [complete YACE permissions list](https://github.com/prometheus-community/yet-another-cloudwatch-exporter?tab=readme-ov-file#authentication).

## Configuration

### Controller Configuration

The operator is configured via the Helm chart values. Key settings:

```yaml
controller:
  serviceAccountName: tagemon-controller-manager
  config:
    tagsHandler:
      viewArn: "arn:aws:resource-explorer-2:REGION:ACCOUNT:view/NAME/ID"  # REQUIRED
      region: "us-west-2"  # REQUIRED
      interval: "1m"
    yaceServiceAccountName: tagemon-yace  # Service account for YACE pods
```

For manual deployments, see [config/controller-config.example.yaml](config/controller-config.example.yaml).

## Tagging Policy Compliance

Tagemon automatically validates AWS resources against tagging policies defined in your Tagemon CRDs. The tagging policy is built from:

- **Name Tag**: Tagemon automatically collects and expects the `Name` tag to be present on all resources. This tag is used to identify resources in metrics and alerts (exposed as the `tag_Name` label in Prometheus metrics).
- **Required Tags**: Tags specified in `exportedTagsOnMetrics` with `required: true` must be present on resources
- **Threshold Tags**: Tags defined in `thresholdTags` must exist and have valid values based on their type (int, bool, percentage)
- **Search Tags**: Resources must match all `searchTags` to be considered for monitoring

### Compliance Metrics

Tagemon exposes the following Prometheus metrics for tagging policy compliance:

- **`tagemon_resources_non_compliant_count`**: A gauge metric tracking the number of non-compliant resources, labeled by:
  - `resource_type`: The AWS resource type (e.g., `rds/db`, `ec2/instance`)
  - `account_id`: The AWS account ID
  - Custom labels: Any labels configured via `nonCompliantMetricCustomLabels` in the controller config

This metric helps you monitor and alert on tagging policy violations across your AWS infrastructure.

### Compliance Checking

The Tags Handler periodically scans AWS resources using AWS Resource Explorer and validates them against the tagging policy. Resources that don't meet the policy requirements are:
- Logged as non-compliant
- Tracked in the `tagemon_resources_non_compliant_count` metric
- Excluded from CloudWatch metric collection until they become compliant

## Architecture

Tagemon consists of three main components:

1. **YACE Handler**: Manages YACE deployments for CloudWatch metrics
2. **Tags Handler**: Validates resource compliance using AWS Resource Explorer and exposes compliance metrics
3. **Config Handler**: Manages controller configuration

## Development

### Prerequisites

- Go 1.24+
- Docker
- kubectl
- kustomize

### Building

```bash
# Generate CRD manifests, RBAC, etc.
make manifests

# Generate code (DeepCopy methods)
make generate

# Build the binary
make build

# Run tests
make test

# Run linter
make lint
```

### Local Development

```bash
# Install CRDs into your cluster
make install

# Run controller locally (outside cluster)
make run

# Uninstall CRDs
make uninstall
```

## Common Operations

### Viewing Resources

```bash
# List all Tagemon resources
kubectl get tagemon -A

# View Tagemon details
kubectl describe tagemon <name> -n tagemon

# Check created YACE deployments
kubectl get deployments -n tagemon -l app.kubernetes.io/created-by=tagemon

# View operator logs
kubectl logs -n tagemon deployment/tagemon-controller-manager -f
```

### Troubleshooting

```bash
# Check operator status
kubectl get pods -n tagemon -l app.kubernetes.io/name=tagemon

# View YACE pod logs
kubectl logs -n tagemon -l app.kubernetes.io/created-by=tagemon

# Check controller metrics
kubectl port-forward -n tagemon svc/tagemon-controller-manager-metrics 8080:8080
curl http://localhost:8080/metrics
```

### Important Notes

- **Namespace Isolation**: Operator watches only the namespace where it's deployed
- **Service Accounts**: Two separate service accounts are created:
  - `tagemon-controller-manager`: For the operator (cluster-scoped)
  - `tagemon-yace`: For YACE pods
- **Resource Management**: Each Tagemon CR creates a dedicated YACE deployment
- **Monitoring**: Requires Prometheus Operator for ServiceMonitor support

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Privacy Policy

For information about how we collect, use, and protect your data, please see our [Privacy Policy](http://nextinsurance.com/privacy-policy/).

## Acknowledgments

- [YACE](https://github.com/nerdswords/yet-another-cloudwatch-exporter) - CloudWatch metrics exporter
- [tag-patrol](https://github.com/eliran89c/tag-patrol) - AWS tag compliance validation
- [Kubebuilder](https://book.kubebuilder.io/) - Kubernetes operator framework

## Support

For issues and questions:
- 📝 [GitHub Issues](https://github.com/next-insurance/tagemon/issues)
- 📖 [Documentation](https://github.com/next-insurance/tagemon/tree/main/docs)

