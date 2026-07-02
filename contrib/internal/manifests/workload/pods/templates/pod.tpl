{{- $name:= .Values.namePattern }}
{{- $namespace:= .Values.namespace }}
{{- $payload:= .Values.payload }}
apiVersion: v1
kind: Pod
metadata:
  name: {{ $name }}
  namespace: {{ $namespace }}
  labels:
    app: fake-pod
spec:
  containers:
    - name: fake-container
      image: fake-image
      env:
        - name: PAYLOAD
          value: {{ $payload | quote }}
