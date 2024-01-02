apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: {{ .Values.name }}
  namespace: {{ .Release.Namespace }}
spec:
  selector:
    matchLabels:
      app: {{ .Values.name }}
  replicas: {{ .Values.replicas }}
  podManagementPolicy: Parallel
  template:
    metadata:
      labels:
        app: {{ .Values.name }}
    spec:
{{- if .Values.controllerNodeSelectors }}
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
  {{- range $key, $values := .Values.controllerNodeSelectors }}
              - key: "{{ $key }}"
                operator: In
                values:
    {{- range $values }}
                  - {{ . }}
    {{- end }}
  {{- end }}
{{- end }}
      terminationGracePeriodSeconds: 1
      containers:
      - args:
        - --config=/data/kwok-config.yaml
        - --manage-all-nodes=false
        - --manage-single-node=$(POD_NAME) # act as virtualnode
        - --disregard-status-with-annotation-selector=kwok.x-k8s.io/status=custom
        - --disregard-status-with-label-selector=
        - --node-ip=$(POD_IP)
        - --node-port=10247
        - --cidr=10.0.0.1/24
        - --node-lease-duration-seconds=40
        env:
        - name: POD_IP
          valueFrom:
            fieldRef:
              fieldPath: status.podIP
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        image: registry.k8s.io/kwok/kwok:v0.4.0
        imagePullPolicy: IfNotPresent
        name: kwok-controller
        volumeMounts:
        - name: kwok-config
          mountPath: /data/
        resources:
         limits:
           cpu: "500m"
         requests:
           cpu: "200m"
      restartPolicy: Always
      serviceAccount: {{ .Values.name }}
      serviceAccountName: {{ .Values.name }}
      volumes:
      - name: kwok-config
        configMap:
          name: {{ .Values.name }}
