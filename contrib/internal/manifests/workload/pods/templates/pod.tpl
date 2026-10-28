{{- $name:= .Values.namePattern }}
{{- $namespace:= .Values.namespace }}
{{- $payload:= .Values.payload }}
{{- $nodeSelector:= .Values.nodeSelector }}
{{- $hasApp := false }}
{{- range $key, $value := .Values.labels }}
{{- if eq $key "app" }}{{ $hasApp = true }}{{ end }}
{{- end }}
apiVersion: v1
kind: Pod
metadata:
  name: {{ printf "%q" $name }}
  namespace: {{ printf "%q" $namespace }}
  labels:
    {{- if not $hasApp }}
    app: fake-pod
    {{- end }}
    {{- range $key, $value := .Values.labels }}
    {{ printf "%q" $key }}: {{ printf "%q" $value }}
    {{- end }}
spec:
  {{- if .Values.serviceAccountName }}
  serviceAccountName: {{ printf "%q" .Values.serviceAccountName }}
  {{- end }}
  {{- if .Values.schedulerName }}
  schedulerName: {{ printf "%q" .Values.schedulerName }}
  {{- end }}
  {{- if $nodeSelector }}
  nodeSelector:
    {{- range $key, $value := $nodeSelector }}
    {{ printf "%q" $key }}: {{ printf "%q" $value }}
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
