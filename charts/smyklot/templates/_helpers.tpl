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
The Secret holding the webhook secret and the App private key.

Required, and named rather than valued: a chart that took either credential
would put it in a values file and in `helm get values` output. Failing here
beats failing at CrashLoopBackOff.
*/}}
{{- define "smyklot.secretName" -}}
{{- required "github.existingSecret is required: create a Secret with the webhook secret and the App private key, then name it here" .Values.github.existingSecret -}}
{{- end -}}
