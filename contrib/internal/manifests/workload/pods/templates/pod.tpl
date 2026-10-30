{{- $name:= .Values.namePattern }}
{{- $namespace:= .Values.namespace }}
{{- $payload:= .Values.payload }}
{{- $nodeSelector:= .Values.nodeSelector }}
apiVersion: v1
kind: Pod
metadata:
  name: {{ $name }}
  namespace: {{ $namespace }}
  labels:
    {{- range $key, $value := .Values.labels }}
    "{{ $key }}": "{{ $value }}"
    {{- end }}
    {{- if eq .Values.labels.app nil }}
    app: fake-pod
    {{- end }}
spec:
  {{- if .Values.serviceAccountName }}
  serviceAccountName: "{{ .Values.serviceAccountName }}"
  {{- end }}
  {{- if .Values.schedulerName }}
  schedulerName: "{{ .Values.schedulerName }}"
  {{- end }}
  {{- if $nodeSelector }}
  nodeSelector:
    {{- range $key, $value := $nodeSelector }}
    "{{ $key }}": "{{ $value }}"
    {{- end }}
  {{- end }}
  # Use pause image so pods reach Running with few events, avoiding an
  # ImagePullBackOff loop that keeps producing events and filling etcd.
  containers:
    - name: fake-container
      image: registry.k8s.io/pause:3.10
      env:
        - name: PAYLOAD
          value: "{{ $payload }}"
