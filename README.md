# AEFTS

Full Text Search system from AppEngine 1st gen. Since it's hard to find a cost effective alternative.

## Run Locally

Install gcloud sdk and app-engine-go component. Refer to github workflow for manual or automated deployment.

```bash
dev_appserver.py app.yaml --log_level=debug --port=8110
```

## Deploy using github workflow

Add these secrets:

```text
APP_ENV - base64 encoded env.yaml using command "cat env.yaml | base64"
GCP_KEY - service account key with deployment roles to appengine also base64
PROJECT_ID - GCP Project
```

Roles to deploy

- App Engine Deployer
- App Engine Service Admin
- Compute Storage Admin
- Cloud Build Service Account
- Cloud Build Editor
- Service Account User
- Viewer

## Docker Image for development

Add to workflow later

```bash
docker build -t altlimit/aefts:v0.0.1 -f build.Dockerfile .
docker push altlimit/aefts:v0.0.1
```
