{{- $name := .Values.name }}
{{- $cpu := .Values.cpu  }}
{{- $memory := .Values.memory }}
{{- $labels := .Values.nodeLabels }}
{{- range $index := (untilStep 0 (int .Values.replicas) 1) }}
apiVersion: v1
kind: Node
metadata:
  annotations:
    node.alpha.kubernetes.io/ttl: "0"
    kwok.x-k8s.io/node: fake
    kwok.x-k8s.io/manage: {{ $name }}-{{ $index }}
  labels:
    beta.kubernetes.io/arch: amd64
    beta.kubernetes.io/os: linux
    kubernetes.io/arch: amd64
    kubernetes.io/hostname: {{ $name }}-{{ $index }}
    kubernetes.io/os: linux
    kubernetes.io/role: agent
    node-role.kubernetes.io/agent: ""
    node.kubernetes.io/exclude-from-external-load-balancers: "true"
    kubernetes.azure.com/managed: "false"
    type: kperf-virtualnodes
    alpha.kperf.io/nodepool: {{ $name }}
{{- range $key, $value := $labels }}
    {{ $key }}: {{ $value }}
{{- end }}
  name: {{ $name }}-{{ $index }}
spec:
  taints: # Avoid scheduling actual running pods to fake Node
  - effect: NoSchedule
    key: kperf.io/nodepool
    value: fake
status:
  allocatable:
    cpu: {{ $cpu }}
    memory: {{ $memory }}Gi
    pods: 110
  capacity:
    cpu: {{ $cpu }}
    memory: {{ $memory }}Gi
    pods: 110
  nodeInfo:
    architecture: amd64
    containerRuntimeVersion: "kwok"
    kubeProxyVersion: fake
    kubeletVersion: fake
    operatingSystem: linux
---
{{- end}}
