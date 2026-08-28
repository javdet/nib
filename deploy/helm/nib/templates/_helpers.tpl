{{/*
Chart name truncated to 63 chars.
*/}}
{{- define "nib.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Fully qualified release name.
*/}}
{{- define "nib.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Chart label value.
*/}}
{{- define "nib.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Component-specific full names.
*/}}
{{- define "nib.backend.fullname" -}}
{{- printf "%s-backend" (include "nib.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "nib.frontend.fullname" -}}
{{- printf "%s-frontend" (include "nib.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "nib.kbMcp.fullname" -}}
{{- printf "%s-kb-mcp" (include "nib.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "nib.postgresql.fullname" -}}
{{- printf "%s-postgresql" (include "nib.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels for all resources.
*/}}
{{- define "nib.labels" -}}
helm.sh/chart: {{ include "nib.chart" . }}
{{ include "nib.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: nib
{{- end }}

{{/*
Selector labels without component.
*/}}
{{- define "nib.selectorLabels" -}}
app.kubernetes.io/name: {{ include "nib.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Labels with component label.
*/}}
{{- define "nib.componentLabels" -}}
{{ include "nib.labels" . }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{/*
Selector labels with component.
*/}}
{{- define "nib.componentSelectorLabels" -}}
{{ include "nib.selectorLabels" . }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{/*
ServiceAccount name.
*/}}
{{- define "nib.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "nib.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Application Secret name.
*/}}
{{- define "nib.secretName" -}}
{{- if .Values.secrets.existingSecret }}
{{- .Values.secrets.existingSecret }}
{{- else }}
{{- printf "%s-app" (include "nib.fullname" .) | trunc 253 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
Backend ConfigMap name.
*/}}
{{- define "nib.backend.configMapName" -}}
{{- printf "%s-backend-config" (include "nib.fullname" .) | trunc 253 | trimSuffix "-" }}
{{- end }}

{{/*
Backend data PVC name.
*/}}
{{- define "nib.backend.pvcName" -}}
{{- printf "%s-backend-data" (include "nib.fullname" .) | trunc 253 | trimSuffix "-" }}
{{- end }}

{{/*
Image tag — defaults to Chart.AppVersion when empty.
*/}}
{{- define "nib.imageTag" -}}
{{- default .Chart.AppVersion .tag }}
{{- end }}

{{/*
PostgreSQL hostname for in-cluster or external database.
*/}}
{{- define "nib.postgresql.host" -}}
{{- if .Values.postgresql.enabled }}
{{- include "nib.postgresql.fullname" . }}
{{- else }}
{{- required "externalDatabase.host is required when postgresql.enabled is false" .Values.externalDatabase.host }}
{{- end }}
{{- end }}

{{/*
PostgreSQL port.
*/}}
{{- define "nib.postgresql.port" -}}
{{- if .Values.postgresql.enabled }}
{{- .Values.postgresql.service.port }}
{{- else }}
{{- .Values.externalDatabase.port }}
{{- end }}
{{- end }}

{{/*
PostgreSQL username.
*/}}
{{- define "nib.postgresql.username" -}}
{{- if .Values.postgresql.enabled }}
{{- .Values.postgresql.auth.username }}
{{- else }}
{{- .Values.externalDatabase.username }}
{{- end }}
{{- end }}

{{/*
PostgreSQL database name.
*/}}
{{- define "nib.postgresql.database" -}}
{{- if .Values.postgresql.enabled }}
{{- .Values.postgresql.auth.database }}
{{- else }}
{{- .Values.externalDatabase.database }}
{{- end }}
{{- end }}

{{/*
PostgreSQL password from values (used in Secret template only).
*/}}
{{- define "nib.postgresql.password" -}}
{{- if .Values.postgresql.enabled }}
{{- .Values.postgresql.auth.password | default .Values.secrets.dbPassword }}
{{- else }}
{{- .Values.externalDatabase.password | default .Values.secrets.dbPassword }}
{{- end }}
{{- end }}

{{/*
PostgreSQL SSL mode.
*/}}
{{- define "nib.postgresql.sslmode" -}}
{{- if .Values.postgresql.enabled }}
{{- .Values.postgresql.auth.sslmode }}
{{- else }}
{{- .Values.externalDatabase.sslmode }}
{{- end }}
{{- end }}

{{/*
PostgreSQL connection URL for kb-mcp.
*/}}
{{- define "nib.postgresql.databaseURL" -}}
{{- $user := include "nib.postgresql.username" . }}
{{- $pass := include "nib.postgresql.password" . }}
{{- $host := include "nib.postgresql.host" . }}
{{- $port := include "nib.postgresql.port" . }}
{{- $db := include "nib.postgresql.database" . }}
{{- $ssl := include "nib.postgresql.sslmode" . }}
{{- printf "postgres://%s:%s@%s:%s/%s?sslmode=%s" $user $pass $host $port $db $ssl }}
{{- end }}

{{/*
Primary ingress host (first configured host).
*/}}
{{- define "nib.ingress.host" -}}
{{- if and .Values.ingress.enabled (gt (len .Values.ingress.hosts) 0) }}
{{- (index .Values.ingress.hosts 0).host }}
{{- else }}
{{- "" }}
{{- end }}
{{- end }}

{{/*
OAuth callback base URL.
*/}}
{{- define "nib.oauth.callbackBaseURL" -}}
{{- if .Values.backend.oauth.callbackBaseURL }}
{{- .Values.backend.oauth.callbackBaseURL }}
{{- else if include "nib.ingress.host" . }}
{{- $scheme := "http" -}}
{{- if gt (len .Values.ingress.tls) 0 -}}
{{- $scheme = "https" -}}
{{- end -}}
{{- printf "%s://%s" $scheme (include "nib.ingress.host" .) }}
{{- else }}
{{- printf "http://%s:%d" (include "nib.backend.fullname" .) (int .Values.backend.service.port) }}
{{- end }}
{{- end }}

{{/*
Frontend base URL for OAuth redirects.
*/}}
{{- define "nib.oauth.frontendBaseURL" -}}
{{- if .Values.backend.oauth.frontendBaseURL }}
{{- .Values.backend.oauth.frontendBaseURL }}
{{- else }}
{{- include "nib.oauth.callbackBaseURL" . }}
{{- end }}
{{- end }}

{{/*
Backend Service URL for frontend nginx API_UPSTREAM.
*/}}
{{- define "nib.backend.serviceURL" -}}
{{- printf "http://%s:%d" (include "nib.backend.fullname" .) (int .Values.backend.service.port) }}
{{- end }}

{{/*
kb-mcp Service URL for mcp.json bootstrap.
*/}}
{{- define "nib.kbMcp.serviceURL" -}}
{{- printf "http://%s:%d/mcp" (include "nib.kbMcp.fullname" .) (int .Values.kbMcp.service.port) }}
{{- end }}

{{/*
ConfigMap checksum annotation for backend pod restarts.
*/}}
{{- define "nib.backend.configChecksum" -}}
{{- include (print $.Template.BasePath "/configmap-backend.yaml") . | sha256sum }}
{{- end }}

{{/*
Default mcp.json content when backend.mcpServers is empty and kbMcp is enabled.
*/}}
{{- define "nib.backend.defaultMcpServers" -}}
{{- if .Values.kbMcp.enabled }}
{{- dict "mcpServers" (dict "knowledge-base" (dict "url" (include "nib.kbMcp.serviceURL" .))) | toJson }}
{{- else }}
{{- dict "mcpServers" dict | toJson }}
{{- end }}
{{- end }}
