# Tagemon

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Tests](https://github.com/next-insurance/tagemon/actions/workflows/tests.yml/badge.svg?branch=main)](https://github.com/next-insurance/tagemon/actions/workflows/tests.yml)
[![Helm Release](https://github.com/next-insurance/tagemon/actions/workflows/helm-release.yml/badge.svg?branch=main)](https://github.com/next-insurance/tagemon/actions/workflows/helm-release.yml)
[![Release](https://github.com/next-insurance/tagemon/actions/workflows/release.yml/badge.svg?branch=main)](https://github.com/next-insurance/tagemon/actions/workflows/release.yml)
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

- Kubernetes cluster (1.24+)
- kubectl configured
- AWS credentials with appropriate permissions

### Installation

```bash
# Install CRDs
kubectl apply -f config/crd/bases/

# Deploy the operator
kubectl apply -f config/default/

# Create a Tagemon resource
kubectl apply -f config/samples/test-cr-ec2.yaml
```

## Usage

Create a Tagemon custom resource to monitor AWS EC2 instances:

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
      thresholdTags:
        - type: percentage
          key: cpu-threshold
          resourceType: instance
          required: true
```

## Configuration

The operator uses a configuration file to set:
- Watch namespace
- Service account name
- Tags handler settings (AWS Resource Explorer integration)

For local development, copy the example config:
```bash
cp config/controller-config.example.yaml config/controller-config.yaml
# Edit with your values
```

See [config/controller-config.example.yaml](config/controller-config.example.yaml) for details.

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
# Build the binary
make build

# Build and push Docker image
make docker-build docker-push IMG=<your-registry>/tagemon:tag

# Run tests
make test

# Run e2e tests
make test-e2e
```

### Local Development

```bash
# Install CRDs
make install

# Run locally
make run
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

### Code of Conduct

Please note that this project is released with a Contributor Code of Conduct. By participating in this project you agree to abide by its terms.

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- [YACE](https://github.com/nerdswords/yet-another-cloudwatch-exporter) - CloudWatch metrics exporter
- [tag-patrol](https://github.com/eliran89c/tag-patrol) - AWS tag compliance validation
- [Kubebuilder](https://book.kubebuilder.io/) - Kubernetes operator framework

## Support

For issues and questions:
- 📝 [GitHub Issues](https://github.com/next-insurance/tagemon/issues)
- 📖 [Documentation](https://github.com/next-insurance/tagemon/tree/main/docs)

