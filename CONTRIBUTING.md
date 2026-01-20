# Contributing to Tagemon

Thank you for your interest in contributing to Tagemon! We welcome contributions from the community.

## Reporting Issues

If you find a bug or have a suggestion, please [open an issue](https://github.com/next-insurance/tagemon/issues) using one of our issue templates. Include:
- A clear description of the problem
- Steps to reproduce (for bugs)
- Your environment (Kubernetes version, AWS region, etc.)
- Relevant logs or error messages

## Submitting Pull Requests

1. **Fork the repository** and create a branch from `main`
   ```bash
   git checkout -b feature/your-feature-name
   ```

2. **Make your changes**:
   - Follow Go best practices and conventions
   - Add tests for new functionality
   - Update documentation as needed
   - Ensure all tests pass (`make test`)

3. **Commit and push**:
   ```bash
   git commit -m "Add feature: description of your change"
   git push origin feature/your-feature-name
   ```

4. **Open a Pull Request** with a clear title and description, referencing any related issues.

## Development Setup

### Prerequisites

- Go 1.24 or later
- Docker
- kubectl configured
- Access to a Kubernetes cluster (for testing)

### Quick Start

```bash
# Clone your fork
git clone https://github.com/your-username/tagemon.git
cd tagemon

# Generate manifests and code
make manifests
make generate

# Build and test
make build
make test

# Run locally
make install
make run
```

## Questions?

If you have questions, please [open an issue](https://github.com/next-insurance/tagemon/issues) with the `question` label or check the [README](README.md) for documentation.

Thank you for contributing! 🎉
