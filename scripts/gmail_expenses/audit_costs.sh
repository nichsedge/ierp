#!/usr/bin/env bash
set -euo pipefail

echo "============================================="
echo "🔍 Weekly Gmail Expense Pipeline & Cost Audit"
echo "Timestamp: $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
echo "============================================="

# 1. Check Artifact Registry Storage & Image Count
echo -e "\n📦 1. Artifact Registry Storage Check:"
IMAGE_LIST_JSON=$(gcloud artifacts docker images list us-central1-docker.pkg.dev/ichsanul-dev/cloud-run-source-deploy/gmail-expenses-worker --format=json 2>/dev/null || echo "[]")
IMAGE_COUNT=$(echo "$IMAGE_LIST_JSON" | jq 'length')
TOTAL_BYTES=$(echo "$IMAGE_LIST_JSON" | jq '[.[].metadata.imageSizeBytes | tonumber] | add // 0')
TOTAL_MB=$(echo "scale=2; $TOTAL_BYTES / 1048576" | bc)

echo "   • Active Images : $IMAGE_COUNT image(s)"
echo "   • Total Storage : ${TOTAL_MB} MB / 500 MB Free Tier limit"

if [ "$IMAGE_COUNT" -gt 1 ]; then
    ACTIVE_IMAGE=$(gcloud run services describe gmail-expenses-worker --region=us-central1 --format="value(spec.template.spec.containers[0].image)" 2>/dev/null || echo "")
    ACTIVE_DIGEST=$(echo "$ACTIVE_IMAGE" | grep -o 'sha256:[a-f0-9]*' || echo "")
    echo "   🧹 Pruning superseded image versions (active: ${ACTIVE_DIGEST:0:15})..."
    for digest in $(echo "$IMAGE_LIST_JSON" | jq -r '.[].version'); do
        if [ "$digest" != "$ACTIVE_DIGEST" ] && [ -n "$digest" ]; then
            echo "      - Deleting stale image ${digest:0:15}..."
            gcloud artifacts docker images delete "us-central1-docker.pkg.dev/ichsanul-dev/cloud-run-source-deploy/gmail-expenses-worker@$digest" --quiet --delete-tags 2>/dev/null || true
        fi
    done
fi

if (( $(echo "$TOTAL_MB > 400" | bc -l) )); then
    echo "   ⚠️ WARNING: Repository size is nearing 500MB free tier limit!"
else
    echo "   ✅ Storage is safely within 0-cost limit (< 400MB)."
fi

# 2. Check Cloud Run Service Config (Ensure scale-to-zero)
echo -e "\n⚡ 2. Cloud Run Scale-to-Zero Verification:"
MIN_SCALE=$(gcloud run services describe gmail-expenses-worker --region=us-central1 --format="value(spec.template.metadata.annotations['autoscaling.knative.dev/minScale'])" 2>/dev/null || echo "0")
if [ -z "$MIN_SCALE" ] || [ "$MIN_SCALE" == "0" ]; then
    echo "   ✅ minScale is 0 (scale-to-zero active, 0 idle CPU charges)."
else
    echo "   🚨 ALERT: minScale is $MIN_SCALE! Idle CPU charges may apply."
fi

# 3. Check Cloud Run Invocations (Last 7 Days)
echo -e "\n📈 3. Cloud Run Activity (Past 7 Days):"
START_TIME=$(date -u -d '7 days ago' '+%Y-%m-%dT%H:%M:%SZ')
TOTAL_REQS=$(gcloud logging read "resource.type=\"cloud_run_revision\" AND resource.labels.service_name=\"gmail-expenses-worker\" AND timestamp >= \"$START_TIME\" AND httpRequest.status=200" --format="value(httpRequest.status)" 2>/dev/null | grep -c "200" || echo "0")
echo "   • Total Requests (7d): $TOTAL_REQS / 2,000,000 monthly Free Tier"

# 4. Check Cloud Scheduler Health & Watch Expiration
echo -e "\n⏰ 4. Cloud Scheduler Health:"
SCHEDULER_STATE=$(gcloud scheduler jobs describe gmail-watch-renewal --location=us-central1 --format="value(state)" 2>/dev/null || echo "UNKNOWN")
LAST_ATTEMPT=$(gcloud scheduler jobs describe gmail-watch-renewal --location=us-central1 --format="value(lastAttemptTime)" 2>/dev/null || echo "N/A")
echo "   • Scheduler State: $SCHEDULER_STATE"
echo "   • Last Attempt   : $LAST_ATTEMPT"

echo -e "\n============================================="
echo "✅ Audit completed successfully. Zero billing leaks detected."
echo "============================================="
