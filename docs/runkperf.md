# runkperf

runkperf is a command-line tool that runs kperf within a Kubernetes cluster to
simulate large workloads and measure the performance and stability of the target kube-apiserver.

## Installing runkperf

See documentation [Getting-Started#Installing-Kperf](/docs/getting-started.md#installing-kperf).

## How to run benchmark test?

runkperf includes three benchmark scenarios, one of which focuses on measuring
performance and stability with 3,000 short-lifecycle pods distributed across 100 nodes.

```bash
$ runkperf bench --runner-image telescope.azurect.io/oss/kperf:v0.1.5 node100_job1_pod3k --help

NAME:
   runkperf bench node100_job1_pod3k -

The test suite is to setup 100 virtual nodes and deploy one job with 3k pods on
that nodes. It repeats to create and delete job. The load profile is fixed.


USAGE:
   runkperf bench node100_job1_pod3k [command options] [arguments...]

OPTIONS:
   --total value         Total requests per runner (There are 10 runners totally and runner's rate is 10) (default: 36000)
   --cpu value           the allocatable cpu resource per node (default: 32)
   --memory value        The allocatable Memory resource per node (GiB) (default: 96)
   --max-pods value      The maximum Pods per node (default: 110)
   --content-type value  Content type (json or protobuf) (default: "json")
```

This test eliminates the need to set up 100 physical nodes, as kperf leverages
[kwok](https://github.com/kubernetes-sigs/kwok) to simulate both nodes and pod
lifecycles. Only a few physical nodes are required to host **5** kperf runners
and **100** kwok controllers.

We **recommend** using two separate node pools in the target Kubernetes cluster
to host the kperf runners and Kwok controllers independently. By default, runkperf
schedules:

* Runners on nodes with instance type: **Standard_D16s_v3** on Azure or **m4.4xlarge** on AWS
* kwok controllers on nodes with instance type: **Standard_D8s_v3** on Azure or **m4.2xlarge** on AWS

You can modify the scheduling affinity for runners and controllers using the 
`--rg-affinity` and `--vc-affinity` options. Please check `runkperf bench --help` for more details.

When that target cluster is ready, you can run

```bash
$ sudo runkperf -v 3 bench \
  --kubeconfig $HOME/.kube/config \
  --runner-image telescope.azurecr.io/oss/kperf:v0.1.5 \
  node100_job1_pod3k --total 1000
```

> NOTE: The `sudo` allows that command to create [mount_namespaces(7)](https://man7.org/linux/man-pages/man7/mount_namespaces.7.html)
to fetch kube-apiserver metrics, for example, `GOMAXPROCS`. However, it's not required.

This command has four steps:

* Setup 100 virtual nodes
* Repeat to create and delete one Job to simulate 3,000 short-lifecycle pods
* Deploy runner group and start measurement
* Retrieve measurement report

You will see that summary when runners finish, like

```bash
{
  "description": "\nEnvironment: 100 virtual nodes managed by kwok-controller,\nWorkload: Deploy 1 job with 3,000 pods repeatedly. The parallelism is 100. The interval is 5s",
  "loadSpec": {
    "count": 10,
    "loadProfile": {
      "version": 1,
      "description": "node100-job1-pod3k",
      "spec": {
        "rate": 10,
        "total": 1000,
        "conns": 10,
        "client": 100,
        "contentType": "json",
        "disableHTTP2": false,
        "maxRetries": 0,
        "Requests": [
          {
            "shares": 1000,
            "staleList": {
              "group": "",
              "version": "v1",
              "resource": "pods",
              "namespace": "",
              "limit": 0,
              "seletor": "",
              "fieldSelector": ""
            }
          },
          {
            "shares": 100,
            "quorumList": {
              "group": "",
              "version": "v1",
              "resource": "pods",
              "namespace": "",
              "limit": 1000,
              "seletor": "",
              "fieldSelector": ""
            }
          },
          {
            "shares": 100,
            "quorumList": {
              "group": "",
              "version": "v1",
              "resource": "events",
              "namespace": "",
              "limit": 1000,
              "seletor": "",
              "fieldSelector": ""
            }
          }
        ]
      }
    },
    "nodeAffinity": {
      "node.kubernetes.io/instance-type": [
        "Standard_D16s_v3",
        "m4.4xlarge"
      ]
    }
  },
  "result": {
    "total": 10000,
    "duration": "1m40.072897445s",
    "errorStats": {
      "unknownErrors": [],
      "netErrors": {},
      "responseCodes": {},
      "http2Errors": {}
    },
    "totalReceivedBytes": 38501695787,
    "percentileLatencies": [
      [
        0,
        0.024862332
      ],
      [
        0.5,
        0.076491594
      ],
      [
        0.9,
        0.135807192
      ],
      [
        0.95,
        0.157084984
      ],
      [
        0.99,
        0.200460794
      ],
      [
        1,
        0.323297381
      ]
    ],
    "percentileLatenciesByURL": {
      "https://10.0.0.1:443/api/v1/events?limit=1000\u0026timeout=1m0s": [
        [
          0,
          0.025955119
        ],
        [
          0.5,
          0.040329283
        ],
        [
          0.9,
          0.05549999
        ],
        [
          0.95,
          0.061468019
        ],
        [
          0.99,
          0.079093604
        ],
        [
          1,
          0.158946761
        ]
      ],
      "https://10.0.0.1:443/api/v1/pods?limit=1000\u0026timeout=1m0s": [
        [
          0,
          0.041545073
        ],
        [
          0.5,
          0.12342483
        ],
        [
          0.9,
          0.186716374
        ],
        [
          0.95,
          0.208233619
        ],
        [
          0.99,
          0.253509952
        ],
        [
          1,
          0.323297381
        ]
      ],
      "https://10.0.0.1:443/api/v1/pods?resourceVersion=0\u0026timeout=1m0s": [
        [
          0,
          0.024862332
        ],
        [
          0.5,
          0.077794907
        ],
        [
          0.9,
          0.131738916
        ],
        [
          0.95,
          0.146966904
        ],
        [
          0.99,
          0.189498717
        ],
        [
          1,
          0.302434749
        ]
      ]
    }
  },
  "info": {
    "apiserver": {
      "cores": {
        "after": {
          "52.167.25.119": 10
        },
        "before": {
          "52.167.25.119": 10
        }
      }
    }
  }
}
```
