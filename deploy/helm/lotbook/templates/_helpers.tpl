{{- define "lotbook.fullname" -}}
{{- if contains .Chart.Name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "lotbook.selector" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "lotbook.labels" -}}
{{ include "lotbook.selector" . }}
app.kubernetes.io/version: {{ .Values.image.tag | trunc 63 | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}

{{- define "lotbook.image" -}}
{{- if .Values.image.digest -}}
{{ .Values.image.repository }}@{{ .Values.image.digest }}
{{- else -}}
{{ .Values.image.repository }}:{{ .Values.image.tag }}
{{- end -}}
{{- end -}}

{{/* The pod-level settings every Lotbook pod shares. */}}
{{- define "lotbook.podSpec" -}}
{{- with .Values.serviceAccountName }}
serviceAccountName: {{ . }}
{{- end }}
automountServiceAccountToken: false
{{- with .Values.imagePullSecrets }}
imagePullSecrets:
  {{- toYaml . | nindent 2 }}
{{- end }}
securityContext:
  runAsNonRoot: true
  runAsUser: 65532
  runAsGroup: 65532
  seccompProfile:
    type: RuntimeDefault
{{- with .Values.nodeSelector }}
nodeSelector:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- with .Values.tolerations }}
tolerations:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- end -}}

{{/* The container-level settings every Lotbook container shares. */}}
{{- define "lotbook.container" -}}
image: {{ include "lotbook.image" . }}
imagePullPolicy: {{ .Values.image.pullPolicy }}
securityContext:
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  capabilities:
    drop: ["ALL"]
{{- end -}}

{{/*
The environment of a container: where the journal is, when it uses Kafka, and
the PostgreSQL URL, when it uses PostgreSQL. A role is given only what it uses.
*/}}
{{- define "lotbook.env" -}}
{{- $ := .root -}}
{{- if .kafka }}
- name: LOTBOOK_KAFKA_BROKERS
  value: {{ $.Values.kafka.brokers | quote }}
- name: LOTBOOK_JOURNAL_TOPIC
  value: {{ $.Values.kafka.journalTopic | quote }}
{{- end }}
{{- if .postgres }}
- name: LOTBOOK_POSTGRES_URL
  valueFrom:
    secretKeyRef:
      name: {{ $.Values.postgres.existingSecret }}
      key: {{ $.Values.postgres.key }}
{{- end }}
- name: LOTBOOK_DRAIN
  value: {{ printf "%ds" (int $.Values.shutdown.drainSeconds) | quote }}
- name: LOTBOOK_GRACE
  value: {{ printf "%ds" (int $.Values.shutdown.graceSeconds) | quote }}
{{- with $.Values.env }}
{{ toYaml . }}
{{- end }}
{{- end -}}

{{/* A long-running role: probes, ports and a stop that drains. */}}
{{- define "lotbook.role" -}}
{{- $ := .root -}}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "lotbook.fullname" $ }}-{{ .role }}
  labels:
    {{- include "lotbook.labels" $ | nindent 4 }}
    app.kubernetes.io/component: {{ .role }}
spec:
  replicas: {{ .values.replicas }}
  selector:
    matchLabels:
      {{- include "lotbook.selector" $ | nindent 6 }}
      app.kubernetes.io/component: {{ .role }}
  template:
    metadata:
      labels:
        {{- include "lotbook.labels" $ | nindent 8 }}
        app.kubernetes.io/component: {{ .role }}
      {{- with $.Values.podAnnotations }}
      annotations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
    spec:
      {{- include "lotbook.podSpec" $ | nindent 6 }}
      terminationGracePeriodSeconds: {{ add $.Values.shutdown.drainSeconds $.Values.shutdown.graceSeconds 5 }}
      topologySpreadConstraints:
        - maxSkew: 1
          topologyKey: kubernetes.io/hostname
          whenUnsatisfiable: ScheduleAnyway
          labelSelector:
            matchLabels:
              {{- include "lotbook.selector" $ | nindent 14 }}
              app.kubernetes.io/component: {{ .role }}
      containers:
        - name: {{ .role }}
          {{- include "lotbook.container" $ | nindent 10 }}
          args: [{{ .role | quote }}{{ range .args }}, {{ . | quote }}{{ end }}]
          env:
            - name: LOTBOOK_ADDR
              value: ":8080"
            {{- include "lotbook.env" (dict "root" $ "kafka" true "postgres" .postgres) | nindent 12 }}
          ports:
            - name: http
              containerPort: 8080
          readinessProbe:
            httpGet: { path: /readyz, port: http }
            periodSeconds: 2
            failureThreshold: 1
          livenessProbe:
            httpGet: { path: /healthz, port: http }
            periodSeconds: 10
            failureThreshold: 3
          resources:
            {{- toYaml .values.resources | nindent 12 }}
{{- if gt (int .values.replicas) 1 }}
---
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: {{ include "lotbook.fullname" $ }}-{{ .role }}
  labels:
    {{- include "lotbook.labels" $ | nindent 4 }}
    app.kubernetes.io/component: {{ .role }}
spec:
  maxUnavailable: 1
  selector:
    matchLabels:
      {{- include "lotbook.selector" $ | nindent 6 }}
      app.kubernetes.io/component: {{ .role }}
{{- end }}
{{- end -}}
