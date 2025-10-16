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

### Basic Installation
```bash
# Install with required configuration (monitoring enabled)
# Controller will automatically watch only the 'default' namespace
helm install tagemon-operator . \
  --set controller.config.tagsHandler.viewArn="arn:aws:resource-explorer-2:us-west-2:123456789012:view/MainView/abc123" \
  --set controller.config.tagsHandler.region="us-west-2"
```

### Minimal Installation
```bash
# Install without monitoring features
helm install tagemon-operator . \
  --set headlessService.enabled=false \
  --set serviceMonitor.enabled=false
```

### Installation Verification

The chart includes automatic verification to ensure the controller is running successfully:

1. **Post-Install Hook**: Automatically runs after installation to verify the controller pod is ready
2. **Post-Upgrade Hook**: Automatically runs after upgrades to verify the controller pod is ready
3. **Helm Tests**: Provides comprehensive health checks you can run manually

```bash
# The post-install and post-upgrade hooks run automatically, but you can also run manual tests:
helm test tagemon-operator -n your-namespace

# Check the status of the verification hooks:
kubectl get pods -l app.kubernetes.io/component=post-install-hook -n your-namespace
kubectl get pods -l app.kubernetes.io/component=post-upgrade-hook -n your-namespace
kubectl logs -l app.kubernetes.io/component=post-install-hook -n your-namespace
kubectl logs -l app.kubernetes.io/component=post-upgrade-hook -n your-namespace

# Disable verification if needed:
helm install tagemon-operator . \
  --set verification.postInstallCheck=false \
  --set verification.enableTests=false
```

## Configuration

### Security Model

The chart creates **two separate service accounts** for enhanced security:

1. **Controller Service Account** (`controller.serviceAccountName`): 
   - Used by the operator controller
   - Has cluster-level permissions to manage Tagemon CRDs, ConfigMaps, and Deployments
   - Requires broader RBAC permissions for its management role

2. **YACE Service Account** (`controller.config.yaceServiceAccountName`):
   - Used by YACE pods created by the operator
   - Has minimal namespace-scoped permissions
   - Designed for AWS IRSA (IAM Roles for Service Accounts) integration
   - Follows principle of least privilege

This separation provides better security isolation and makes it easier to configure AWS IAM roles for YACE pods without over-privileging the controller.

### Controller Configuration

The controller watches a specific namespace for Tagemon Custom Resources based on the `watchNamespace` configuration. **The Helm chart automatically sets this to the deployment namespace** (`{{ .Release.Namespace }}`), providing namespace isolation without manual configuration.

**Namespace Configuration:**
- **Helm deployments**: Automatically set to `{{ .Release.Namespace }}`
- **Direct deployments**: Must configure `watchNamespace` in config file (REQUIRED)
- **Empty/missing value**: Controller will fail to start

```yaml
controller:
  # Deployment configuration
  replicas: 1
  image:
    repository: amitshlo/amit-tag
    tag: v1.0.0
  
  # Log level for the controller (debug, info, error)
  logLevel: "info"
  
  resources:
    limits:
      cpu: 500m
      memory: 128Mi
    requests:
      cpu: 10m
      memory: 64Mi
  
  # Service account for the controller
  serviceAccountName: tagemon-dev-controller-manager
  
  # Runtime configuration
  config:
    # Service account for YACE pods created by the operator
    yaceServiceAccountName: tagemon-dev-yace
    
    tagsHandler:
      viewArn: "arn:aws:resource-explorer-2:us-west-2:123456789012:view/MainView/abc123"  # REQUIRED
      region: "us-west-2"  # REQUIRED
      interval: "1m"
```

### Custom Resources

#### Headless Service (for YACE service discovery)
```yaml
headlessService:
  enabled: true  # Enable for production
  port: 5000
  targetPort: 5000
```

#### ServiceMonitor (for Prometheus monitoring)
```yaml
serviceMonitor:
  enabled: true  # Enable if using Prometheus Operator
  interval: 30s  # Scrape interval
  labels:
    prometheus: kube-prometheus  # Match your Prometheus selector
```

## Usage Examples

### Development Setup
```yaml
# values-dev.yaml
controllerManager:
  container:
    image:
      tag: latest

# Both monitoring features enabled by default
```

### Production Setup
```yaml
# values-prod.yaml
controllerManager:
  replicas: 1
  container:
    image:
      tag: "v1.0.0"  # Use specific version
    resources:
      limits:
        cpu: 1000m
        memory: 256Mi

headlessService:
  enabled: true

serviceMonitor:
  enabled: true
  interval: 30s
  labels:
    prometheus: kube-prometheus
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
  metrics:
    - name: CPUUtilization
  statistics:
    - Average
  period: 300
  length: 3600
```

## Values Reference

| Parameter | Description | Default |
|-----------|-------------|---------|
| `controller.replicas` | Number of controller replicas | `1` |
| `controller.image.repository` | Controller image repository | `amitshlo/amit-tag` |
| `controller.image.tag` | Controller image tag | `v1.0.0` |
| `controller.logLevel` | Log level for the controller (debug, info, error) | `info` |
| `controller.resources` | Controller resource limits/requests | See values.yaml |
| `controller.serviceAccountName` | Service account name for controller | `tagemon-dev-controller-manager` |
| `controller.config.yaceServiceAccountName` | Service account name for YACE pods | `tagemon-dev-yace` |
| `controller.config.tagsHandler.viewArn` | AWS Resource Explorer view ARN (REQUIRED) | `""` |
| `controller.config.tagsHandler.region` | AWS region for Resource Explorer (REQUIRED) | `us-east-1` |
| `controller.config.tagsHandler.interval` | Tag polling interval | `1m` |
| `rbac.enable` | Enable RBAC resources | `true` |
| `crd.enable` | Enable CRD installation | `true` |
| `crd.keep` | Keep CRDs when uninstalling | `false` |
| `verification.postInstallCheck` | Enable post-install and post-upgrade verification hooks | `true` |
| `verification.enableTests` | Enable Helm test resources | `true` |
| `metrics.enable` | Enable operator metrics service | `true` |
| `headlessService.enabled` | Enable headless service for YACE pods | `true` |
| `serviceMonitor.enabled` | Enable ServiceMonitor for Prometheus | `true` |

## Troubleshooting

### Check Operator Status
```bash
kubectl get deployment tagemon-dev-controller-manager
kubectl logs deployment/tagemon-dev-controller-manager
```

### Check Tagemon Resources
```bash
kubectl get tagemons
kubectl describe tagemon <name>
```

### Check Created Resources
```bash
# ConfigMaps created by operator
kubectl get configmaps -l app.kubernetes.io/created-by=tagemon

# Deployments created by operator  
kubectl get deployments -l app.kubernetes.io/created-by=tagemon
```

## Uninstalling

```bash
# Remove the Helm release (keeps CRDs by default)
helm uninstall tagemon-operator

# To also remove CRDs (this will delete all Tagemon resources!)
kubectl delete crd tagemons.tagemon.io
```

## Support

For issues and questions:
- Check the operator logs
- Verify RBAC permissions
- Ensure AWS credentials are properly configured for the operator