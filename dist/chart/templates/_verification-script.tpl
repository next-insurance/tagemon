{{/*
Shared verification script for post-install and post-upgrade hooks
*/}}
{{- define "chart.verificationScript" -}}
echo "🚀 {{ .hookType }} verification starting..."
echo "================================================"

# Wait for deployment rollout
echo "⏳ Waiting for controller deployment to be ready..."
kubectl rollout status deployment/{{ include "chart.name" . }}-controller-manager \
  --namespace={{ .Release.Namespace }} \
  --timeout=120s

if [ $? -ne 0 ]; then
  echo "❌ Controller deployment failed to roll out"
  exit 1
fi

# Verify pod is running and ready
echo "🔍 Verifying controller pod status..."
POD_NAME=$(kubectl get pods -l control-plane=controller-manager \
  --namespace={{ .Release.Namespace }} \
  -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)

if [ -z "$POD_NAME" ]; then
  echo "❌ No controller pod found"
  exit 1
fi

echo "📋 Controller pod: $POD_NAME"

# Wait for pod to be ready
kubectl wait --for=condition=ready pod/$POD_NAME \
  --namespace={{ .Release.Namespace }} \
  --timeout=60s

if [ $? -eq 0 ]; then
  echo "✅ Controller pod is ready!"
else
  echo "❌ Controller pod failed to become ready"
  kubectl describe pod/$POD_NAME --namespace={{ .Release.Namespace }}
  exit 1
fi

# Optional: Check if controller is responding to health checks
echo "🔍 Checking controller health endpoint..."
kubectl exec $POD_NAME --namespace={{ .Release.Namespace }} -- \
  wget -q -O- http://localhost:8081/healthz 2>/dev/null || echo "⚠️  Health check not accessible (this is normal during startup)"

echo "================================================"
echo "✅ {{ .hookType }} verification completed successfully!"
echo "🎉 Tagemon operator is ready to use!"
{{- end -}}
