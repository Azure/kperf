# Testing kperf

This document provides instructions for testing and using kperf to benchmark Kubernetes API servers.

## Using kperf

### kperf runner run

The `kperf runner run` command generates requests from the endpoint where the command is executed. All requests are generated based on a load profile configuration.

Example load profile (`/tmp/example-loadprofile.yaml`):

```yaml
version: 1
description: example profile
spec:
  # rate defines the maximum requests per second (zero is no limit).
  rate: 100

  # total defines the total number of requests.
  total: 10

  # conns defines total number of individual transports used for traffic.
  conns: 100

  # client defines total number of HTTP clients. These clients share connection
  # pool represented by `conns` field.
  client: 1000

  # contentType defines response's content type. (json or protobuf)
  contentType: json

  # disableHTTP2 means client will use HTTP/1.1 protocol if it's true.
  disableHTTP2: false

  # pick up requests randomly based on defined weight.
  requests:
    # staleList means this list request with zero resource version.
    - staleList:
        version: v1
        resource: pods
      shares: 1000 # Has 50% chance = 1000 / (1000 + 1000)
    # quorumList means this list request without kube-apiserver cache.
    - quorumList:
        version: v1
        resource: pods
        limit: 1000
      shares: 1000 # Has 50% chance = 1000 / (1000 + 1000)
```

This profile generates two types of requests:
- **stale list**: `/api/v1/pods` (cached responses)
- **quorum list**: `/api/v1/pods?limit=1000` (bypasses cache)

Run the test:

```bash
kperf -v 3 runner run --config /tmp/example-loadprofile.yaml
```

The result shows percentile latencies and provides latency details for each request type.

> **Note**: Use `kperf runner run -h` to see more options.

### kperf runnergroup

The `kperf runnergroup` command manages a group of runners within a target Kubernetes cluster. Each runner is deployed as an individual Pod, allowing distributed load generation from multiple endpoints.

#### Deploy runners

Example runner group spec (`/tmp/example-runnergroup-spec.yaml`):

```yaml
# count defines how many runners in the group.
count: 10

# loadProfile defines what the load traffic looks like.
# All runners in this group will use the same load profile.
loadProfile:
  version: 1
  description: example profile
  spec:
    rate: 100
    total: 10
    conns: 100
    client: 1000
    contentType: json
    disableHTTP2: false
    requests:
      - staleList:
          version: v1
          resource: pods
        shares: 1000
      - quorumList:
          version: v1
          resource: pods
          limit: 1000
        shares: 1000

# nodeAffinity defines how to deploy runners into dedicated nodes.
nodeAffinity:
  node.kubernetes.io/instance-type:
    - n1-standard-16
```

Deploy the runner group:

```bash
kperf rg run \
  --runner-image=ghcr.io/azure/kperf:0.3.4 \
  --runnergroup="file:///tmp/example-runnergroup-spec.yaml"
```

> **Note**: Uses URI schemes to load specs. Supports `file://absolute-path` and `configmap://name?namespace=ns&specName=dataNameInCM`.

#### Check status

```bash
kperf rg status
```

#### Get results

```bash
kperf rg result --wait
```

The `--wait` flag blocks until all runners finish.

#### Delete runners

```bash
kperf rg delete
```

### kperf virtualcluster nodepool

The `nodepool` subcommand uses [kwok](https://github.com/kubernetes-sigs/kwok) to deploy virtual nodepools, allowing simulation of 1,000+ node scenarios with minimal physical resources.

#### Add nodepool

```bash
kperf vc nodepool add example \
  --nodes=10 --cpu=32 --memory=96 --max-pods=50 \
  --affinity="node.kubernetes.io/instance-type=n1-standard-16"
```

#### Schedule pods to virtual nodes

To schedule pods on virtual nodes, use these affinity and toleration settings:

```yaml
affinity:
  nodeAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      nodeSelectorTerms:
      - matchExpressions:
        - key: type
          operator: In
          values:
          - kperf-virtualnodes

tolerations:
- key: "kperf.io/nodepool"
  operator: "Exists"
  effect: "NoSchedule"
```

#### List nodepools

```bash
kperf vc nodepool list
```

#### Delete nodepool

```bash
kperf vc nodepool delete example
```

## Important Notes

- Runner groups use Helm releases deployed in the `runnergroups-kperf-io` namespace
- Virtual nodes are managed in the `virtualnodes-kperf-io` namespace
- By default, job controller pods complete after 5 seconds; other pods run until deleted
- Only one long-running server is allowed per cluster currently