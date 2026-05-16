{{/*
Common labels applied to every resource. Caller passes a dict with:
  - Root: the chart's root context ($)
  - Name: the service name
*/}}
{{- define "iicpc.labels" -}}
app.kubernetes.io/name: {{ .Name }}
app.kubernetes.io/part-of: iicpc-platform
app.kubernetes.io/managed-by: helm
helm.sh/chart: {{ printf "%s-%s" .Root.Chart.Name .Root.Chart.Version | replace "+" "_" }}
{{- end -}}

{{/*
Selector labels (subset that must NOT change across releases).
*/}}
{{- define "iicpc.selectorLabels" -}}
app.kubernetes.io/name: {{ .Name }}
app.kubernetes.io/part-of: iicpc-platform
{{- end -}}

{{/*
Resolve per-service config by merging defaults + per-service overrides.
Caller: include "iicpc.serviceConfig" (dict "defaults" $.Values.defaults "service" $svc)
*/}}
{{- define "iicpc.replicasMin" -}}
{{- $svc := .service -}}
{{- $defaults := .defaults -}}
{{- if and $svc.replicas $svc.replicas.min -}}{{ $svc.replicas.min }}{{- else -}}{{ $defaults.replicas.min }}{{- end -}}
{{- end -}}

{{- define "iicpc.replicasMax" -}}
{{- $svc := .service -}}
{{- $defaults := .defaults -}}
{{- if and $svc.replicas $svc.replicas.max -}}{{ $svc.replicas.max }}{{- else -}}{{ $defaults.replicas.max }}{{- end -}}
{{- end -}}
