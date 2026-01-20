# Tagemon

[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)
[![Tests](https://github.com/next-insurance/tagemon/actions/workflows/tests.yml/badge.svg?branch=main)](https://github.com/next-insurance/tagemon/actions/workflows/tests.yml)
[![Helm Release](https://github.com/next-insurance/tagemon/actions/workflows/helm-release.yml/badge.svg?branch=main)](https://github.com/next-insurance/tagemon/actions/workflows/helm-release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/next-insurance/tagemon)](https://goreportcard.com/report/github.com/next-insurance/tagemon)

A Kubernetes operator for monitoring AWS CloudWatch metrics with tag-based compliance validation.

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
  serviceAccountName: tagemon-controller
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
  --namespace monitoring \
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

### Basic Example

```yaml
apiVersion: tagemon.io/v1alpha1
kind: Tagemon
metadata:
  name: ec2-monitor
  namespace: monitoring
spec:
  type: AWS/EC2
  regions:
    - us-east-1
  awsRoles:
    - roleArn: arn:aws:iam::123456789012:role/TagemonRole
  statistics:
    - Average
  period: 300
  metrics:
    - name: CPUUtilization
      statistics:
        - Average
```

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
| `podResources` | object | - | Resource requests/limits for YACE pods |
| `namePrefix` | string | - | Prefix for generated resource names (max 200 chars) |

### AWS Roles

Configure IAM roles for CloudWatch access:

```yaml
awsRoles:
  - roleArn: arn:aws:iam::123456789012:role/TagemonRole
```

**Fields:**
- `roleArn` (required): IAM role ARN (pattern: `arn:aws:iam::ACCOUNT:role/NAME`)

### Metrics

Define which CloudWatch metrics to collect:

```yaml
metrics:
  - name: CPUUtilization
    statistics: [Average, Maximum]  # Optional: overrides spec.statistics
    period: 60                       # Optional: overrides spec.period
    nilToZero: true                  # Optional: overrides spec.nilToZero
    addCloudwatchTimestamp: false    # Optional: overrides spec.addCloudwatchTimestamp
    thresholdTags:                   # Optional: tag-based compliance
      - type: percentage
        key: cpu-threshold
        resourceType: instance
        required: true
```

**Fields:**
- `name` (required): CloudWatch metric name
- `statistics` (optional): Override default statistics (`Sum`, `Average`, `Maximum`, `Minimum`, `SampleCount`)
- `period` (optional): Override default period (min: 2 seconds)
- `nilToZero` (optional): Override default nil-to-zero behavior
- `addCloudwatchTimestamp` (optional): Override timestamp behavior
- `thresholdTags` (optional): Define tag-based thresholds for compliance validation

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

Configure resource requests and limits for YACE pods:

```yaml
podResources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 500m
    memory: 256Mi
```

### Complete Example

```yaml
apiVersion: tagemon.io/v1alpha1
kind: Tagemon
metadata:
  name: rds-monitoring
  namespace: monitoring
spec:
  type: AWS/RDS
  regions:
    - us-east-1
    - us-west-2
  
  awsRoles:
    - roleArn: arn:aws:iam::123456789012:role/TagemonRole
  
  # Global defaults
  statistics: [Average, Maximum]
  period: 300
  scrapingInterval: 120
  nilToZero: true
  
  # Filter only production databases
  searchTags:
    - key: Environment
      value: production
  
  # Export tags as Prometheus labels
  exportedTagsOnMetrics:
    - key: Environment
      required: true
    - key: DatabaseName
      required: false
  
  # Metrics to collect
  metrics:
    - name: CPUUtilization
      statistics: [Average, Maximum]
      thresholdTags:
        - type: percentage
          key: cpu-threshold
          resourceType: database
          required: true
    
    - name: DatabaseConnections
      statistics: [Average, Sum]
    
    - name: FreeableMemory
      period: 60  # More frequent collection
      nilToZero: false
  
  # Resource limits for YACE pods
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
  serviceAccountName: tagemon-controller
  config:
    tagsHandler:
      viewArn: "arn:aws:resource-explorer-2:REGION:ACCOUNT:view/NAME/ID"  # REQUIRED
      region: "us-west-2"  # REQUIRED
      interval: "1m"
    yaceServiceAccountName: tagemon-yace  # Service account for YACE pods
```

For manual deployments, see [config/controller-config.example.yaml](config/controller-config.example.yaml).

## Architecture

Tagemon consists of three main components:

1. **YACE Handler**: Manages YACE deployments for CloudWatch metrics
2. **Tags Handler**: Validates resource compliance using AWS Resource Explorer
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
kubectl describe tagemon <name> -n monitoring

# Check created YACE deployments
kubectl get deployments -n monitoring -l app.kubernetes.io/created-by=tagemon

# View operator logs
kubectl logs -n monitoring deployment/tagemon-controller-manager -f
```

### Troubleshooting

```bash
# Check operator status
kubectl get pods -n monitoring -l app.kubernetes.io/name=tagemon

# View YACE pod logs
kubectl logs -n monitoring -l app.kubernetes.io/created-by=tagemon

# Check controller metrics
kubectl port-forward -n monitoring svc/tagemon-controller-manager-metrics 8080:8080
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

