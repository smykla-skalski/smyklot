{{/* Chart name, overridable */}}
{{- define "smyklot.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Fully qualified release name */}}
{{- define "smyklot.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "smyklot.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "smyklot.labels" -}}
helm.sh/chart: {{ include "smyklot.chart" . }}
{{ include "smyklot.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "smyklot.selectorLabels" -}}
app.kubernetes.io/name: {{ include "smyklot.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "smyklot.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "smyklot.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "smyklot.image" -}}
{{- printf "%s:%s" .Values.image.repository (default .Chart.AppVersion .Values.image.tag) -}}
{{- end -}}

{{/*
Fail at template time rather than at CrashLoopBackOff.

Both of these are required and neither has a sensible default: without App
credentials there is nothing to act as, and without a webhook secret anyone who
can reach the port could drive the bot.
*/}}
{{- define "smyklot.validate" -}}
{{- if not .Values.github.clientId -}}
{{- fail "github.clientId is required: set it to the App's client ID (or its numeric app ID)" -}}
{{- end -}}
{{- if not .Values.github.existingSecret -}}
{{- fail "github.existingSecret is required: create a Secret with the webhook secret and the App private key, then name it here" -}}
{{- end -}}
{{- if and .Values.ingress.enabled (not .Values.ingress.host) -}}
{{- fail "ingress.host is required when ingress.enabled is true" -}}
{{- end -}}
{{- end -}}
