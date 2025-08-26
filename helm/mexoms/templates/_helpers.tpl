{{/*
Expand the name of the chart.
*/}}
{{- define "mexoms.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "mexoms.fullname" -}}
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
Create chart name and version as used by the chart label.
*/}}
{{- define "mexoms.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "mexoms.labels" -}}
helm.sh/chart: {{ include "mexoms.chart" . }}
{{ include "mexoms.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "mexoms.selectorLabels" -}}
app.kubernetes.io/name: {{ include "mexoms.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "mexoms.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "mexoms.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
PostgreSQL fullname
*/}}
{{- define "mexoms.postgresql.fullname" -}}
{{- if .Values.postgresql.fullnameOverride -}}
{{- .Values.postgresql.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default "postgresql" .Values.postgresql.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Redis fullname
*/}}
{{- define "mexoms.redis.fullname" -}}
{{- if .Values.redis.fullnameOverride -}}
{{- .Values.redis.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default "redis" .Values.redis.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- printf "%s-master" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s-master" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
NATS fullname
*/}}
{{- define "mexoms.nats.fullname" -}}
{{- if .Values.nats.fullnameOverride -}}
{{- .Values.nats.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default "nats" .Values.nats.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Database environment variables
*/}}
{{- define "mexoms.env.database" -}}
- name: DATABASE_HOST
  value: {{ include "mexoms.postgresql.fullname" . }}
- name: DATABASE_PORT
  value: "5432"
- name: DATABASE_NAME
  value: {{ .Values.postgresql.auth.database }}
- name: DATABASE_USER
  value: {{ .Values.postgresql.auth.username }}
- name: DATABASE_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ include "mexoms.fullname" . }}-secrets
      key: postgres-password
{{- end }}

{{/*
Redis environment variables
*/}}
{{- define "mexoms.env.redis" -}}
- name: REDIS_HOST
  value: {{ include "mexoms.redis.fullname" . }}
- name: REDIS_PORT
  value: "6379"
- name: REDIS_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ include "mexoms.fullname" . }}-secrets
      key: redis-password
{{- end }}

{{/*
NATS environment variables
*/}}
{{- define "mexoms.env.nats" -}}
- name: NATS_URL
  value: nats://{{ include "mexoms.nats.fullname" . }}:4222
- name: NATS_CLUSTER_ID
  value: mexoms-cluster
{{- end }}

{{/*
Security environment variables
*/}}
{{- define "mexoms.env.security" -}}
- name: JWT_SECRET
  valueFrom:
    secretKeyRef:
      name: {{ include "mexoms.fullname" . }}-secrets
      key: jwt-secret
- name: VAULT_TOKEN
  valueFrom:
    secretKeyRef:
      name: {{ include "mexoms.fullname" . }}-secrets
      key: vault-token
{{- end }}