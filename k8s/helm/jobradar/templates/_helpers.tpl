{{/*
Common labels applied to all resources.
*/}}
{{- define "jobradar.labels" -}}
app: jobradar
environment: {{ .Values.environment }}
{{- end }}

{{/*
Selector labels for a given component.
Usage: include "jobradar.selectorLabels" (dict "component" "api")
*/}}
{{- define "jobradar.selectorLabels" -}}
app: jobradar
component: {{ .component }}
{{- end }}

{{/*
Standard security context for Go worker containers (non-root).
*/}}
{{- define "jobradar.workerSecurityContext" -}}
runAsNonRoot: true
runAsUser: 65532
runAsGroup: 65532
readOnlyRootFilesystem: true
allowPrivilegeEscalation: false
capabilities:
  drop: ["ALL"]
{{- end }}
