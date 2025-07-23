{{- $pattern := .Values.namePattern }}
{{- $jobCount := int .Values.jobCount }}
{{- $podsPerJob := int .Values.podsPerJob }}
{{- $parallelism := int .Values.parallelism }}
{{- $namespace := .Values.namespace }}
{{- range $index := (untilStep 0 (int .Values.jobCount) 1) }}
---
apiVersion: batch/v1
kind: Job
metadata:
  name: {{ $pattern }}-{{ $index }}
  namespace: {{ $namespace }}
  labels:
    app: kperf-benchmark
    job-group: {{ $pattern }}
    job-index: "{{ $index }}"
spec:
  completions: {{ $podsPerJob }}
  parallelism: {{ $parallelism }}
  backoffLimit: 3
  template:
    metadata:
      labels:
        app: fake-pod
        job: {{ $pattern }}-{{ $index }}
        job-group: {{ $pattern }}
        job-index: "{{ $index }}"
    spec:
      restartPolicy: Never
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
      containers:
      - name: fake-container
        image: fake-image
{{- end }}
