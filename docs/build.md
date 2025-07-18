# Building and Installing kperf

This document provides instructions for building, testing, and installing the kperf benchmarking tool.

## Building and Testing

### Build all binaries
Creates `bin/kperf` and `bin/contrib/runkperf`:
```bash
make build
```

### Run tests
```bash
make test
```

### Run linting
```bash
make lint
```

### Clean build artifacts
```bash
make clean
```

### Install binaries
Installs to `/usr/local/bin` (or use `PREFIX=/path make install` for custom location):
```bash
make install
```

## Container Operations

### Build container image
```bash
make image-build
```

### Push container image
```bash
make image-push
```

### Clean container image
```bash
make image-clean
```
