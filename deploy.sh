#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_FILE="${SCRIPT_DIR}/deploy.conf"

# Check if config file exists
if [ ! -f "$CONFIG_FILE" ]; then
    echo "Error: deploy.conf not found!"
    echo "Copy deploy.conf.example to deploy.conf and fill in your credentials:"
    echo "  cp deploy.conf.example deploy.conf"
    exit 1
fi

# Source the config file
source "$CONFIG_FILE"

# Validate required variables
if [ -z "$DOCKER_USERNAME" ] || [ "$DOCKER_USERNAME" = "your_dockerhub_username" ]; then
    echo "Error: DOCKER_USERNAME not set in deploy.conf"
    exit 1
fi

if [ -z "$DOCKER_TOKEN" ] || [ "$DOCKER_TOKEN" = "your_dockerhub_access_token" ]; then
    echo "Error: DOCKER_TOKEN not set in deploy.conf"
    exit 1
fi

# Set defaults
DOCKER_IMAGE_NAME="${DOCKER_IMAGE_NAME:-instastream}"
DOCKER_TAG="${DOCKER_TAG:-latest}"

FULL_IMAGE_NAME="${DOCKER_USERNAME}/${DOCKER_IMAGE_NAME}:${DOCKER_TAG}"

echo "==> Building Docker image: ${FULL_IMAGE_NAME}"
docker build -t "$FULL_IMAGE_NAME" "$SCRIPT_DIR"

echo "==> Logging in to Docker Hub"
echo "$DOCKER_TOKEN" | docker login -u "$DOCKER_USERNAME" --password-stdin

echo "==> Pushing image to Docker Hub"
docker push "$FULL_IMAGE_NAME"

echo "==> Logging out from Docker Hub"
docker logout

echo ""
echo "==> Successfully pushed: ${FULL_IMAGE_NAME}"

# Update Lambda if configured
if [ -n "$LAMBDA_FUNCTION_NAME" ] && [ "$LAMBDA_FUNCTION_NAME" != "your_lambda_function_name" ]; then
    AWS_REGION="${AWS_REGION:-us-east-1}"
    
    echo ""
    echo "==> Updating Lambda function: ${LAMBDA_FUNCTION_NAME}"
    
    aws lambda update-function-code \
        --function-name "$LAMBDA_FUNCTION_NAME" \
        --image-uri "docker.io/${FULL_IMAGE_NAME}" \
        --region "$AWS_REGION"
    
    echo "==> Waiting for Lambda update to complete..."
    aws lambda wait function-updated \
        --function-name "$LAMBDA_FUNCTION_NAME" \
        --region "$AWS_REGION"
    
    echo "==> Lambda function updated successfully!"
else
    echo ""
    echo "Skipping Lambda update (LAMBDA_FUNCTION_NAME not configured)"
    echo "To enable, set LAMBDA_FUNCTION_NAME in deploy.conf"
fi

echo ""
echo "To pull and run locally:"
echo "  docker pull ${FULL_IMAGE_NAME}"
echo "  docker run -p 8080:8080 ${FULL_IMAGE_NAME}"
