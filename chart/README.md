# Tagemon Operator Helm Chart

This Helm chart deploys the Tagemon Operator for managing AWS CloudWatch metrics collection via YACE (Yet Another CloudWatch Exporter) with tag-based compliance validation.

## Overview

The Tagemon Operator creates and manages YACE deployments based on Tagemon custom resources, enabling dynamic CloudWatch metrics collection with proper service discovery and monitoring integration.

### What Gets Deployed

**Core Components:**
- **Deployment**: Tagemon operator controller
- **ServiceAccount**: For operator permissions (`tagemon-controller-manager`)
- **ClusterRole/ClusterRoleBinding**: RBAC for managing Tagemons, ConfigMaps, and Deployments
- **CustomResourceDefinition**: Tagemon CRD
- **Service**: Operator metrics endpoint

**Monitoring Components (enabled by default):**
- **Headless Service**: For YACE pod service discovery
- **ServiceMonitor**: For Prometheus monitoring of YACE pods and controller

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
  serviceAccountName: tagemon-controller-manager
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
  --namespace tagemon \
  --create-namespace \
  -f values.yaml
```

Or install from local chart:

```bash
helm install tagemon ./chart \
  --namespace tagemon \
  --create-namespace \
  -f values.yaml
```

### Installation Verification

The chart includes automatic verification:

```bash
# Run Helm tests
helm test tagemon -n tagemon

# Check verification hooks
kubectl get pods -n tagemon -l app.kubernetes.io/component=post-install-hook
kubectl logs -n tagemon -l app.kubernetes.io/component=post-install-hook
```

## AWS Prerequisites

Before installing, ensure you have:
- AWS Resource Explorer configured with a view ARN
- IAM roles configured for both service accounts

See the [AWS Prerequisites section](../README.md#aws-prerequisites) in the main README for detailed setup instructions and IAM permission requirements.

## Configuration

### Controller Configuration

Key configuration values:

```yaml
controller:
  replicas: 1
  
  image:
    repository: nextinsurancedevops/tagemon
    tag: v0.0.20
  
  logLevel: "info"  # debug, info, error
  
  serviceAccountName: tagemon-controller-manager
  
  config:
    yaceServiceAccountName: tagemon-yace
    
    tagsHandler:
      viewArn: "arn:aws:resource-explorer-2:us-west-2:ACCOUNT:view/NAME/ID"  # REQUIRED
      region: "us-west-2"  # REQUIRED
      interval: "60m"
      nonCompliantMetricCustomLabels: {}  # Optional: Custom labels for compliance metrics
    
    yace:
      customImage: ""  # Optional: Custom YACE image (defaults to prometheuscommunity/yet-another-cloudwatch-exporter:v0.62.1)
  
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
# ServiceMonitor for Prometheus Operator (controller metrics)
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

After installing the operator, create Tagemon resources to start monitoring. 

📖 **See the [main README](../README.md#tagemon-crd-specification) for:**
- Complete CRD field reference
- Full examples with threshold tags, search tags, and exported tags
- Tagging policy compliance details
- Alerting examples

## Values Reference

| Parameter | Description | Default |
|-----------|-------------|---------|
| `controller.replicas` | Number of controller replicas | `1` |
| `controller.image.repository` | Controller image repository | `nextinsurancedevops/tagemon` |
| `controller.image.tag` | Controller image tag | `v0.0.20` |
| `controller.logLevel` | Log level (debug, info, error) | `info` |
| `controller.serviceAccountName` | Service account for controller | `tagemon-controller-manager` |
| `controller.config.yaceServiceAccountName` | Service account for YACE pods | `tagemon-yace` |
| `controller.config.tagsHandler.viewArn` | AWS Resource Explorer view ARN (REQUIRED) | `""` |
| `controller.config.tagsHandler.region` | AWS region (REQUIRED) | `us-west-2` |
| `controller.config.tagsHandler.interval` | Tag polling interval | `1m` |
| `controller.config.tagsHandler.nonCompliantMetricCustomLabels` | Custom labels for compliance metrics | `{}` |
| `controller.config.yace.customImage` | Custom YACE image | `""` |
| `controller.resources` | Controller resource limits/requests | See values.yaml |
| `controller.serviceMonitor.enabled` | Enable controller ServiceMonitor | `true` |
| `yaceServiceMonitor.enabled` | Enable YACE ServiceMonitor | `true` |
| `headlessService.enabled` | Enable headless service for YACE pods | `true` |
| `rbac.enable` | Enable RBAC resources | `true` |
| `verification.postInstallCheck` | Enable verification hooks | `true` |
| `verification.enableTests` | Enable Helm tests | `true` |
| `metrics.enable` | Enable metrics service | `true` |

## Troubleshooting

```bash
# Check operator status
kubectl get pods -n tagemon -l app.kubernetes.io/name=tagemon
kubectl logs -n tagemon deployment/tagemon-controller-manager -f

# Check Tagemon resources
kubectl get tagemon -n tagemon
kubectl describe tagemon <name> -n tagemon

# Check created YACE deployments
kubectl get deployments -n tagemon -l app.kubernetes.io/created-by=tagemon

# View YACE pod logs
kubectl logs -n tagemon -l app.kubernetes.io/created-by=tagemon

# Check controller metrics
kubectl port-forward -n tagemon svc/tagemon-controller-manager-metrics 8080:8080
curl http://localhost:8080/metrics
```

## Upgrading

```bash
# Upgrade to new version
helm upgrade tagemon oci://docker.io/nextinsurancedevops/tagemon \
  --namespace tagemon \
  -f values.yaml

# Upgrade with new values
helm upgrade tagemon oci://docker.io/nextinsurancedevops/tagemon \
  --namespace tagemon \
  --set controller.image.tag=v0.0.21 \
  --reuse-values
```

## Uninstalling

```bash
# Remove the Helm release (CRDs preserved by default)
helm uninstall tagemon -n tagemon

# To also remove CRDs (WARNING: Deletes all Tagemon resources!)
kubectl delete crd tagemons.tagemon.io

# Clean up remaining resources
kubectl delete configmaps -n tagemon -l app.kubernetes.io/created-by=tagemon
kubectl delete deployments -n tagemon -l app.kubernetes.io/created-by=tagemon
```

**⚠️ Important:** By default, CRDs are not deleted to prevent accidental data loss.

## Additional Documentation

- [Complete CRD Field Reference](../README.md#tagemon-crd-specification)
- [Tagging Policy Compliance](../README.md#tagging-policy-compliance)
- [Threshold Tags and Alerting](../README.md#threshold-tags)
- [Development Guide](../README.md#development)
- [YACE Documentation](https://github.com/prometheus-community/yet-another-cloudwatch-exporter)
