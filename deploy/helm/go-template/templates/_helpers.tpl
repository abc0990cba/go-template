{{/* Standard name helpers. */}}
{{- define "go-template.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "go-template.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name (include "go-template.name" .) | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{- define "go-template.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{ include "go-template.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "go-template.selectorLabels" -}}
app.kubernetes.io/name: {{ include "go-template.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "go-template.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "go-template.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/* The workload kind for the api: Deployment by default, Rollout when
     progressive delivery is enabled (values.rollout.enabled). The HPA
     target switches with it. */}}
{{- define "go-template.apiWorkloadKind" -}}
{{- if .Values.rollout.enabled }}Rollout{{- else }}Deployment{{- end }}
{{- end }}

{{/* The api pod template, shared verbatim by the Deployment and the
     Rollout so the two shapes can never drift. */}}
{{- define "go-template.api.podSpec" -}}
metadata:
  labels:
    {{- include "go-template.selectorLabels" . | nindent 4 }}
    app.kubernetes.io/component: api
spec:
  serviceAccountName: {{ include "go-template.serviceAccountName" . }}
  {{- with .Values.imagePullSecrets }}
  imagePullSecrets:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  securityContext:
    runAsNonRoot: {{ .Values.securityContext.runAsNonRoot }}
    runAsUser: {{ .Values.securityContext.runAsUser }}
  containers:
    - name: api
      image: "{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}"
      imagePullPolicy: {{ .Values.image.pullPolicy }}
      securityContext:
        readOnlyRootFilesystem: {{ .Values.securityContext.readOnlyRootFilesystem }}
        allowPrivilegeEscalation: {{ .Values.securityContext.allowPrivilegeEscalation }}
        capabilities:
          drop:
            {{- toYaml .Values.securityContext.capabilities.drop | nindent 12 }}
      args: ["/app/api"]
      ports:
        - name: http
          containerPort: 8080
      env:
        {{- include "go-template.env" . | nindent 8 }}
        {{- range $key, $value := .Values.api.env }}
        - name: {{ $key }}
          value: {{ $value | quote }}
        {{- end }}
      startupProbe:
        httpGet: { path: /health/startup, port: http }
        periodSeconds: 2
        failureThreshold: 30
      livenessProbe:
        httpGet: { path: /health/live, port: http }
        periodSeconds: {{ .Values.probe.periodSeconds }}
      readinessProbe:
        httpGet: { path: /health/ready, port: http }
        periodSeconds: {{ .Values.probe.periodSeconds }}
      {{- with .Values.api.resources }}
      resources:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- if .Values.secrets.data }}
      volumeMounts:
        - name: secrets
          mountPath: /etc/go-template-secrets
          readOnly: true
      {{- end }}
  {{- if .Values.secrets.data }}
  volumes:
    - name: secrets
      secret:
        secretName: {{ .Values.secrets.existingSecret | default (include "go-template.fullname" .) }}
  {{- end }}
{{- end }}

{{/* Build the APP_* env list shared by api and worker. */}}
{{- define "go-template.env" -}}
- name: APP_HTTP_ADDR
  value: ":8080"
- name: APP_DATABASE_URL
  value: {{ .Values.config.databaseURL | quote }}
- name: APP_REDIS_ADDR
  value: {{ .Values.config.redisAddr | quote }}
- name: APP_LOG_FORMAT
  value: {{ .Values.config.logFormat | quote }}
- name: APP_COOKIE_SECURE
  value: {{ .Values.config.cookieSecure | quote }}
{{- if .Values.config.clickhouseAddr }}
- name: APP_CLICKHOUSE_ADDR
  value: {{ .Values.config.clickhouseAddr | quote }}
{{- end }}
{{- if .Values.config.otlpEndpoint }}
- name: APP_OTLP_ENDPOINT
  value: {{ .Values.config.otlpEndpoint | quote }}
{{- end }}
{{- if .Values.config.otlpSampler }}
- name: APP_OTLP_SAMPLER
  value: {{ .Values.config.otlpSampler | quote }}
{{- end }}
{{- range $key, $value := .Values.config.extraEnv }}
- name: {{ $key }}
  value: {{ $value | quote }}
{{- end }}
{{- range $key, $value := .Values.secrets.data }}
{{- if hasSuffix "_file" $key }}
{{/* File-carried secrets: mount path wired through the <VAR>_FILE pattern. */}}
{{- $var := trimSuffix "_file" (printf "%s" $key) }}
- name: {{ $var }}_FILE
  value: /etc/go-template-secrets/{{ $key }}
{{- else }}
- name: {{ $key }}
  valueFrom:
    secretKeyRef:
      name: {{ include "go-template.fullname" $ }}
      key: {{ $key }}
{{- end }}
{{- end }}
{{- end }}
