# kperf - a kube-apiserver benchmark tool

kperf is a benchmarking tool for the Kubernetes API server that allows users to
conduct high-load testing on simulated clusters. Its primary purpose is to emulate
clusters larger than the actual environment, helping to uncover potential control
plane issues based on the user's workload scale. This tool provides an efficient,
cost-effective way for users to validate the performance and stability of their
Kubernetes API server.

# Why kperf?

kperf offers unique advantages over tools like kubemark by simulating a broader
range of traffic patterns found in real Kubernetes workloads. While kubemark
primarily emulates kubelet traffic, kperf can replicate complex interactions
typically associated with controllers, operators, and daemonsets. This includes
scenarios like stale list requests from the API server cache, quorum-based list
operations that directly impact etcd, and informer cache lists and watch behaviors.
By covering these additional traffic types, kperf provides a more comprehensive
view of control plane performance and stability, making it an essential tool for
understanding how a cluster will handle high-load scenarios across diverse workload patterns.

## Getting Started

The `kperf` commands provide low-level tools to create and manage virtual node pools, and to benchmark a target kube-apiserver using customizable load profile YAML files.

See documentation on [kperf.md](/docs/kperf.md) for more detail.

## Running in Cluster

The `runkperf` commands provide high-level orchestration to run end-to-end benchmarks on a target Kubernetes cluster.

See documentation on [runkperf](/docs/runkperf.md) for more detail.

## Contributing

This project welcomes contributions and suggestions.  Most contributions require you to agree to a
Contributor License Agreement (CLA) declaring that you have the right to, and actually do, grant us
the rights to use your contribution. For details, visit https://cla.opensource.microsoft.com.

When you submit a pull request, a CLA bot will automatically determine whether you need to provide
a CLA and decorate the PR appropriately (e.g., status check, comment). Simply follow the instructions
provided by the bot. You will only need to do this once across all repos using our CLA.

This project has adopted the [Microsoft Open Source Code of Conduct](https://opensource.microsoft.com/codeofconduct/).
For more information see the [Code of Conduct FAQ](https://opensource.microsoft.com/codeofconduct/faq/) or
contact [opencode@microsoft.com](mailto:opencode@microsoft.com) with any additional questions or comments.

## Trademarks

This project may contain trademarks or logos for projects, products, or services. Authorized use of Microsoft
trademarks or logos is subject to and must follow
[Microsoft's Trademark & Brand Guidelines](https://www.microsoft.com/en-us/legal/intellectualproperty/trademarks/usage/general).
Use of Microsoft trademarks or logos in modified versions of this project must not cause confusion or imply Microsoft sponsorship.
Any use of third-party trademarks or logos are subject to those third-party's policies.
