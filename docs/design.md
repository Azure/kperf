## Overview

kperf is a benchmarking tool for Kubernetes API servers that simulates high-load testing on clusters. It provides two main binaries:

- `kperf`: Low-level functions for measuring kube-apiserver performance
- `runkperf`: High-level benchmark scenarios that combine kperf functions

## Architecture

### Main Components

1. **kperf CLI** (`cmd/kperf/`): Main command-line interface
   - `runner/`: Manages load generation from single endpoint
   - `runnergroup/`: Manages distributed load generation across cluster
   - `virtualcluster/`: Manages virtual nodes using kwok

2. **runkperf CLI** (`contrib/cmd/runkperf/`): Higher-level benchmark scenarios
   - `bench/`: Pre-configured benchmark scenarios (e.g., node10_job1_pod100)
   - `data/`: Data generation commands for configmaps, daemonsets
   - `warmup/`: Cluster warmup operations

3. **Core Libraries**:
   - `api/types/`: Core data structures (LoadProfile, RunnerGroup, etc.)
   - `request/`: HTTP client and request generation logic
   - `runner/`: Runner group management and orchestration
   - `virtualcluster/`: Virtual node lifecycle management
   - `manifests/`: Helm chart templates and Kubernetes manifests

### Request Types

kperf supports different types of API requests:
- **staleList**: List requests with resourceVersion=0 (cached responses)
- **quorumList**: List requests that bypass cache and hit etcd
- **watch**: Watch requests for real-time updates
- **get**: Individual resource retrieval

### Load Profiles

Load profiles define traffic patterns in YAML format with:
- Rate limiting (requests per second)
- Connection pooling configuration
- Client distribution
- Request type weighting (shares-based)
- Content type (JSON or protobuf)

### Runner Groups

Runner groups deploy multiple pods across cluster nodes to generate distributed load:
- Managed via Helm releases in `runnergroups-kperf-io` namespace
- Each runner executes same load profile
- Port forwarding used for communication with control server
- Node affinity controls runner placement

### Virtual Clusters

Virtual nodes are created using kwok to simulate large clusters:
- Managed in `virtualnodes-kperf-io` namespace
- Supports custom node specifications (CPU, memory, max pods)
- Requires specific node affinity and tolerations for pod scheduling
