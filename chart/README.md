# Tagemon Operator Helm Chart

This Helm chart deploys the Tagemon Operator for managing AWS CloudWatch metrics collection via YACE (Yet Another CloudWatch Exporter).

## Overview

The Tagemon Operator creates and manages YACE deployments based on Tagemon custom resources, enabling dynamic CloudWatch metrics collection with proper service discovery and monitoring integration.

### What Gets Deployed

**Core Components:**
- **Deployment**: Tagemon operator controller
- **ServiceAccount**: For operator permissions
- **ClusterRole/ClusterRoleBinding**: RBAC for managing Tagemons, ConfigMaps, and Deployments
- **CustomResourceDefinition**: Tagemon CRD
- **Service**: Operator metrics endpoint

**Monitoring Components (enabled by default):**
- **Headless Service**: For YACE pod service discovery
- **ServiceMonitor**: For Prometheus monitoring of YACE pods

## Quick Start

### Prerequisites

- Kubernetes cluster (1.24+)
- kubectl configured
- Helm 3.x
- AWS credentials with appropriate permissions
- AWS Resource Explorer configured with a view ARN

### Installation

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

Install from OCI registry:

```bash
helm install tagemon oci://docker.io/nextinsurancedevops/tagemon \
  --namespace monitoring \
  --create-namespace \
  -f values.yaml
```

Or install from local chart:

```bash
helm install tagemon ./chart \
  --namespace monitoring \
  --create-namespace \
  -f values.yaml
```

### Installation Verification

The chart includes automatic verification:

```bash
# Run Helm tests
helm test tagemon -n monitoring

# Check verification hooks
kubectl get pods -n monitoring -l app.kubernetes.io/component=post-install-hook
kubectl logs -n monitoring -l app.kubernetes.io/component=post-install-hook
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

### Service Accounts

The chart creates **two separate service accounts** for enhanced security:

1. **Controller Service Account** (`tagemon-controller`): 
   - Used by the operator controller
   - Needs cluster-level permissions to manage Tagemon CRDs, ConfigMaps, and Deployments
   - Requires `resource-explorer-2:Search` permission for tag compliance validation

2. **YACE Service Account** (`tagemon-yace`):
   - Used by YACE pods created by the operator
   - Needs permissions to assume YACE roles and collect CloudWatch metrics
   - Should be annotated with IRSA for AWS access
   - Follows principle of least privilege

### Controller Configuration

Key configuration values:

```yaml
controller:
  replicas: 1
  
  image:
    repository: nextinsurancedevops/tagemon
    tag: v0.0.10
  
  logLevel: "info"  # debug, info, error
  
  serviceAccountName: tagemon-controller
  
  config:
    yaceServiceAccountName: tagemon-yace
    
    tagsHandler:
      viewArn: "arn:aws:resource-explorer-2:us-west-2:ACCOUNT:view/NAME/ID"  # REQUIRED
      region: "us-west-2"  # REQUIRED
      interval: "60m"
  
  resources:
    limits:
      cpu: 500m
      memory: 128Mi
    requests:
      cpu: 10m
      memory: 64Mi
```

**Note:** The controller automatically watches only the namespace where it's deployed (`{{ .Release.Namespace }}`).

### Monitoring

Configure Prometheus integration:

```yaml
# ServiceMonitor for Prometheus Operator
controller:
  serviceMonitor:
    enabled: true
    interval: 60s
    labels:
      owner: platform-engineering

# ServiceMonitor for YACE pods
yaceServiceMonitor:
  enabled: true
  interval: 60s
  labels:
    owner: platform-engineering

# Headless service for YACE pod discovery
headlessService:
  enabled: true
  port: 5000
```

## Creating Tagemon Resources

After installing the operator, create Tagemon resources to start monitoring:

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
  statistics:
    - Average
  period: 300
  metrics:
    - name: CPUUtilization
    - name: DatabaseConnections
```

See the [main README](../README.md#tagemon-crd-specification) for complete CRD field reference.

## Values Reference

| Parameter | Description | Default |
|-----------|-------------|---------|
| `controller.replicas` | Number of controller replicas | `1` |
| `controller.image.repository` | Controller image repository | `nextinsurancedevops/tagemon` |
| `controller.image.tag` | Controller image tag | `v0.0.10` |
| `controller.logLevel` | Log level (debug, info, error) | `info` |
| `controller.serviceAccountName` | Service account for controller | `tagemon-controller-manager` |
| `controller.config.yaceServiceAccountName` | Service account for YACE pods | `tagemon-yace` |
| `controller.config.tagsHandler.viewArn` | AWS Resource Explorer view ARN (REQUIRED) | `""` |
| `controller.config.tagsHandler.region` | AWS region (REQUIRED) | `us-west-2` |
| `controller.config.tagsHandler.interval` | Tag polling interval | `1m` |
| `controller.resources` | Controller resource limits/requests | See values.yaml |
| `controller.serviceMonitor.enabled` | Enable controller ServiceMonitor | `true` |
| `yaceServiceMonitor.enabled` | Enable YACE ServiceMonitor | `true` |
| `headlessService.enabled` | Enable headless service for YACE pods | `true` |
| `rbac.enable` | Enable RBAC resources | `true` |
| `verification.postInstallCheck` | Enable verification hooks | `true` |

## Troubleshooting

```bash
# Check operator status
kubectl get pods -n monitoring -l app.kubernetes.io/name=tagemon
kubectl logs -n monitoring deployment/tagemon-controller-manager -f

# Check Tagemon resources
kubectl get tagemon -n monitoring
kubectl describe tagemon <name> -n monitoring

# Check created YACE deployments
kubectl get deployments -n monitoring -l app.kubernetes.io/created-by=tagemon

# Check controller metrics
kubectl port-forward -n monitoring svc/tagemon-controller-manager-metrics 8080:8080
curl http://localhost:8080/metrics
```

## Upgrading

```bash
# Upgrade to new version
helm upgrade tagemon oci://docker.io/nextinsurancedevops/tagemon \
  --namespace monitoring \
  -f values.yaml

# Upgrade with new values
helm upgrade tagemon oci://docker.io/nextinsurancedevops/tagemon \
  --namespace monitoring \
  --set controller.image.tag=v0.0.11 \
  --reuse-values
```

## Uninstalling

```bash
# Remove the Helm release (CRDs preserved by default)
helm uninstall tagemon -n monitoring

# To also remove CRDs (WARNING: Deletes all Tagemon resources!)
kubectl delete crd tagemons.tagemon.io

# Clean up remaining resources
kubectl delete configmaps -n monitoring -l app.kubernetes.io/created-by=tagemon
kubectl delete deployments -n monitoring -l app.kubernetes.io/created-by=tagemon
```

**⚠️ Important:** By default, CRDs are not deleted to prevent accidental data loss.

## Additional Documentation

- [Complete CRD Field Reference](../README.md#tagemon-crd-specification)
- [Development Guide](../README.md#development)
- [YACE Documentation](https://github.com/prometheus-community/yet-another-cloudwatch-exporter)