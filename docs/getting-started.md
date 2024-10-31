# Getting started with kperf

## Installing kperf

Currently, kperf hasn't released official binary yet. To install kperf, we need
to build kperf from source.

### Build requirements

The following build system dependencies are required:

* Go 1.22.X or above
* Linux platform
* GNU Make
* Git

> NOTE: The contrib/cmd/runkperf binary is using [mount_namespaces(7)](https://man7.org/linux/man-pages/man7/mount_namespaces.7.html)
to fetch metrics from each instance of kube-apiserver. It requires Linux platform.

### Build kperf

You need git to checkout the source code:

```bash
git clone https://github.com/Azure/kperf.git
```

`kperf` uses `make` to create a repeatable build flow.
That means you can run:

```bash
cd kperf
make
```

This is going to build binaries in the `./bin` directory.

You can move them in your `PATH`. You can run:

```bash
sudo make install
```

By default, the binaries will be in `/usr/local/bin`. The install prefix can be
changed by passing the `PREFIX` variable (default: `/usr/local`).

## Using kperf

### kperf-runner run

The `kperf runner run` command generates requests from the endpoint where the command is executed.
This command provides flexiable way to configure how to generate requests to Kubernetes API server.
All the requests are generated based on load profile, for example.

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

  # client defines total number of HTTP clients. These clients shares connection
  # pool represented by `conn:` field.
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

Let's see what that profile means here.

There are two kinds of requests and all the responses are in JSON format.

* stale list: `/api/v1/pods`
* quorum list: `/api/v1/pods?limit=1000`

That command will send out `10` requests with `100` QPS as maximum rate.
You can adjust the `total` and `rate` fields to control the test duration.

Before generating requests, that comamnd will generate `100` individual connections and share them in `1000` clients.

When the number of clients exceeds the available connections, each client selects a specific connection based on its index.
The goal is for each client to select a connection based on its index to ensure every client is assigned a connection in a round-robin fashion.

```plain
Client 0 is assigned to Connection 0
Client 1 is assigned to Connection 1
Client 2 is assigned to Connection 2
Client 3 is assigned to Connection 0
Client 4 is assigned to Connection 1
```

The above profile is located at `/tmp/example-loadprofile.yaml`. You can run

```bash
$ kperf -v 3 runner run --config /tmp/example-loadprofile.yaml
I1028 23:08:18.948632  294624 schedule.go:96] "Setting" clients=1000 connections=100 rate=100 total=10 http2=true content-type="json"
{
  "total": 10,
  "duration": "367.139837ms",
  "errorStats": {
    "unknownErrors": [],
    "netErrors": {},
    "responseCodes": {},
    "http2Errors": {}
  },
  "totalReceivedBytes": 2856450,
  "percentileLatencies": [
    [
      0,
      0.235770565
    ],
    [
      0.5,
      0.247910802
    ],
    [
      0.9,
      0.266660525
    ],
    [
      0.95,
      0.286721785
    ],
    [
      0.99,
      0.286721785
    ],
    [
      1,
      0.286721785
    ]
  ],
  "percentileLatenciesByURL": {
    "https://xyz:443/api/v1/pods?limit=1000\u0026timeout=1m0s": [
      [
        0,
        0.235770565
      ],
      [
        0.5,
        0.245662504
      ],
      [
        0.9,
        0.266660525
      ],
      [
        0.95,
        0.266660525
      ],
      [
        0.99,
        0.266660525
      ],
      [
        1,
        0.266660525
      ]
    ],
    "https://xyz:443/api/v1/pods?resourceVersion=0\u0026timeout=1m0s": [
      [
        0,
        0.23650554
      ],
      [
        0.5,
        0.247910802
      ],
      [
        0.9,
        0.286721785
      ],
      [
        0.95,
        0.286721785
      ],
      [
        0.99,
        0.286721785
      ],
      [
        1,
        0.286721785
      ]
    ]
  }
}
```

The result shows the percentile latencies and also provides latency details based on each kind of request.

> NOTE: Please checkout `kperf runner run -h` to see more options.

If you want to run benchmark in Kubernetes cluster, please use `kperf runnergroup`.

### kperf-runnergroup

The `kperf runnergroup` command manages a group of runners within a target Kubernetes cluster. 
A runner group consists of multiple runners, with each runner deployed as an individual Pod for the `kperf runner` process.
These runners not only generate requests within the cluster but can also issue requests from multiple endpoints,
mitigating limitations such as network bandwidth constraints.

#### run - deploy a set of runners into kubernetes

Each runner in a group shares the same load profile. For example, it's defination about runner group.
There are 10 runners in one group and they will be scheduled to `Standard_DS2_v2` type nodes.

```yaml
# count defines how many runners in the group.
count: 10

# loadProfile defines what the load traffic looks like.
# All the runners in this group will use the same load profile.
loadProfile:
  version: 1
  description: example profile
  spec:
    # rate defines the maximum requests per second (zero is no limit).
    rate: 100

    # total defines the total number of requests.
    total: 10

    # conns defines total number of individual transports used for traffic.
    conns: 100

    # client defines total number of HTTP clients.
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

# nodeAffinity defines how to deploy runners into dedicated nodes which have specific labels.
nodeAffinity:
  node.kubernetes.io/instance-type:
    - Standard_DS2_v2
```

Let's say the local file `/tmp/example-runnergroup-spec.yaml`. You can run:

```bash
$ kperf rg run \
  --runner-image=telescope.azurecr.io/oss/kperf:v0.1.5 \
  --runnergroup="file:///tmp/example-runnergroup-spec.yaml"
```

We use URI scheme to load runner group's spec.
For example, `file://absolute-path`. We also support read spec from configmap, `configmap://name?namespace=ns&specName=dataNameInCM`.
Please checkout `kperf rg run -h` to see more options.

> NOTE: Currently, we use helm release to deploy a long running sever as controller to
deploy runners. The namespace is `runnergroups-kperf-io` and we don't allow run
multiple long running servers right now.

#### status - check runner group's status

After deploy runner groups successfully, you can use `status` to check.

```bash
$ kperf rg status
NAME                   COUNT   SUCCEEDED   FAILED   STATE      START
runnergroup-server-0   10      10          0        finished   2024-10-29T00:30:03Z
```

#### result - wait for test report

We use `result` to wait for report.

```bash
$ kperf rg result --wait
{
  "total": 100,
  "duration": "283.369368ms",
  "errorStats": {
    "unknownErrors": [],
    "netErrors": {},
    "responseCodes": {},
    "http2Errors": {}
  },
  "totalReceivedBytes": 36087700,
  "percentileLatencies": [
    [
      0,
      0.031640566
    ],
    [
      0.5,
      0.084185705
    ],
    [
      0.9,
      0.152182422
    ],
    [
      0.95,
      0.172522186
    ],
    [
      0.99,
      0.186271132
    ],
    [
      1,
      0.205396874
    ]
  ],
  "percentileLatenciesByURL": {
    "https://10.0.0.1:443/api/v1/pods?limit=1000\u0026timeout=1m0s": [
      [
        0,
        0.044782901
      ],
      [
        0.5,
        0.093048564
      ],
      [
        0.9,
        0.152182422
      ],
      [
        0.95,
        0.174676524
      ],
      [
        0.99,
        0.205396874
      ],
      [
        1,
        0.205396874
      ]
    ],
    "https://10.0.0.1:443/api/v1/pods?resourceVersion=0\u0026timeout=1m0s": [
      [
        0,
        0.031640566
      ],
      [
        0.5,
        0.076792273
      ],
      [
        0.9,
        0.158094428
      ],
      [
        0.95,
        0.172522186
      ],
      [
        0.99,
        0.176899664
      ],
      [
        1,
        0.176899664
      ]
    ]
  }
}
```

> NOTE: `--wait` is used to block until all the runners finished.

#### delete - delete runner groups

```bash
$ kperf rg delete
```

### kperf-virtualcluster nodepool

The `nodepool` subcmd is using [kwok](https://github.com/kubernetes-sigs/kwok) to
deploy virtual nodepool. You can use few physical resources to simulate more than 1,000 nodes scenario.

> NOTE: The `kperf` uses `virtualnodes-kperf-io` namespace to host resources related to nodepool.

If the user wants to schedule pods to virtual nodes, the user needs to change node affinity and tolerations for pods.

```YAML
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

Be default, the pod created by job controller will be completed after 5 seconds.
Other pods will be long running until receiving delete event.

#### add - add a set of nodes with the same setting

You can use the following command to add nodepool named by `example` with 10 nodes.

```bash
$ kperf vc nodepool add example \
  --nodes=10 --cpu=32 --memory=96 --max-pods=50 \
  --affinity="node.kubernetes.io/instance-type=Standard_DS2_v2"
```

> NOTE: The `--affinity` is used to deploy node controller (kwok) to nodes with the specific labels.

You can use `kubectl get nodes` to check.

```bash
$ kubectl get nodes -o wide | grep example
example-0                           Ready    agent    21s   fake      10.244.15.21   <none>        <unknown>            kwok-v0.5.1         kwok
example-1                           Ready    agent    21s   fake      10.244.13.18   <none>        <unknown>            kwok-v0.5.1         kwok
example-2                           Ready    agent    21s   fake      10.244.14.18   <none>        <unknown>            kwok-v0.5.1         kwok
example-3                           Ready    agent    21s   fake      10.244.15.22   <none>        <unknown>            kwok-v0.5.1         kwok
example-4                           Ready    agent    21s   fake      10.244.13.19   <none>        <unknown>            kwok-v0.5.1         kwok
example-5                           Ready    agent    21s   fake      10.244.14.21   <none>        <unknown>            kwok-v0.5.1         kwok
example-6                           Ready    agent    21s   fake      10.244.14.20   <none>        <unknown>            kwok-v0.5.1         kwok
example-7                           Ready    agent    21s   fake      10.244.14.19   <none>        <unknown>            kwok-v0.5.1         kwok
example-8                           Ready    agent    21s   fake      10.244.13.20   <none>        <unknown>            kwok-v0.5.1         kwok
example-9                           Ready    agent    21s   fake      10.244.15.23   <none>        <unknown>            kwok-v0.5.1         kwok
```

#### list - list all the existing nodepools

```bash
$ kperf vc nodepool list
NAME         NODES     CPU   MEMORY (GiB)   MAX PODS   STATUS
example      ? / 10    32    96             50         deployed
```

> NOTE: There is TODO item to show the number of ready nodes. Before that, we
use `?` as read nodes.

#### delete - delete the target nodepool

```bash
$ kperf vc nodepool delete example
$
$ kperf vc nodepool list
NAME         NODES    CPU   MEMORY (GiB)   MAX PODS   STATUS
```
