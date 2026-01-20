---
name: Bug Report
about: Create a report to help us improve
title: '[BUG] '
labels: bug
assignees: ''
---

## Description

A clear and concise description of what the bug is.

## Steps to Reproduce

1. 
2. 
3. 
4. 

## Expected Behavior

A clear description of what you expected to happen.

## Actual Behavior

A clear description of what actually happened.

## Environment

Please provide the following information:

- **Kubernetes Version**: 
- **Tagemon Version**: 
- **AWS Region(s)**: 
- **Cloud Provider**: (e.g., EKS, GKE, AKS, on-premises)
- **Go Version** (if building from source): 
- **Helm Version** (if using Helm): 

## Configuration

If applicable, please share your Tagemon CRD configuration (redact any sensitive information):

```yaml
# Your Tagemon resource configuration
```

## Logs

Please include relevant logs or error messages:

```
# Controller logs
kubectl logs -n <namespace> deployment/tagemon-controller-manager

# YACE pod logs (if applicable)
kubectl logs -n <namespace> -l app.kubernetes.io/created-by=tagemon
```

## Additional Context

Add any other context, screenshots, or information about the problem here.

