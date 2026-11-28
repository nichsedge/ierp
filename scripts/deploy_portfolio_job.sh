#!/usr/bin/env bash
set -euo pipefail

PROJECT_ID="${GCP_PROJECT_ID:-$(gcloud config get-value project)}"
REGION="us-central1"
JOB_NAME="ierp-portfolio"
SCHEDULER_NAME="ierp-portfolio-monthly"

echo "Deploying Cloud Run Job '${JOB_NAME}' in ${REGION} (project: ${PROJECT_ID})..."
gcloud run jobs deploy "${JOB_NAME}" \
  --source . \
  --region "${REGION}" \
  --memory 512Mi \
  --cpu 1 \
  --max-retries 1 \
  --task-timeout 5m

echo "Configuring monthly Cloud Scheduler trigger (25th or next Monday at 17:00 WIB)..."
PROJECT_NUMBER="$(gcloud projects describe "${PROJECT_ID}" --format='value(projectNumber)')"
JOB_URI="https://run.googleapis.com/v2/projects/${PROJECT_ID}/locations/${REGION}/jobs/${JOB_NAME}:run"
SERVICE_ACCOUNT="${PROJECT_NUMBER}-compute@developer.gserviceaccount.com"

if gcloud scheduler jobs describe "${SCHEDULER_NAME}" --location "${REGION}" >/dev/null 2>&1; then
  gcloud scheduler jobs update http "${SCHEDULER_NAME}" \
    --location "${REGION}" \
    --schedule "0 17 25-27 * *" \
    --time-zone "Asia/Jakarta" \
    --uri "${JOB_URI}" \
    --http-method POST \
    --oauth-service-account-email "${SERVICE_ACCOUNT}"
else
  gcloud scheduler jobs create http "${SCHEDULER_NAME}" \
    --location "${REGION}" \
    --schedule "0 17 25-27 * *" \
    --time-zone "Asia/Jakarta" \
    --uri "${JOB_URI}" \
    --http-method POST \
    --oauth-service-account-email "${SERVICE_ACCOUNT}"
fi

echo "Done! Set secrets/env vars on the Cloud Run Job via:"
echo "  gcloud run jobs update ${JOB_NAME} --region ${REGION} --set-env-vars=ALCHEMY_API_KEY=...,SOL_ADDRESS=...,ETH_ADDRESS=...,R2_ACCOUNT_ID=...,R2_ACCESS_KEY_ID=...,R2_SECRET_ACCESS_KEY=...,R2_BUCKET_NAME=...,TELEGRAM_BOT_TOKEN=...,TELEGRAM_CHAT_ID=..."
