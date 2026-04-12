#!/bin/bash
set -e

PROJECT_ID="xzerolab-480008"
SERVICE_NAME="agent-combined-svc-plus"
IMAGE_NAME="asia-northeast1-docker.pkg.dev/$PROJECT_ID/cloud-run-source-deploy/agent.svc.plus/agent-svc-plus:latest"

# 1. Build and Push Agent Image
echo "Building and pushing agent image..."
docker build -f Dockerfile.xhttp -t $IMAGE_NAME .
docker push $IMAGE_NAME

# 2. Deploy to Regions
REGIONS=("asia-east2" "us-west1")

for REGION in "${REGIONS[@]}"; do
    echo "Deploying to $REGION..."
    
    # Note: Cloud Run services are region-bound. 
    # For specific zones like asia-east2-b, Cloud Run handles zone placement automatically within the region.
    
    gcloud run services replace deploy/gcp/cloud-run/agent-sidecar-service.yaml \
        --platform managed \
        --region $REGION \
        --project $PROJECT_ID
done

echo "Deployment complete."
