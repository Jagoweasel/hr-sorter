# Deployment Instructions

Follow these steps to deploy the application to a Kubernetes cluster.

## 1. Prepare Secrets
Edit `k8s/secret.yaml` with your credentials encoded in base64.
You can use the following command to encode values:
```bash
echo -n "your_value" | base64
```
Apply the secret:
```bash
kubectl apply -f k8s/secret.yaml
```

## 2. Persistent Storage
Apply the Persistent Volume Claim for SQLite:
```bash
kubectl apply -f k8s/pvc.yaml
```

## 3. Deploy Application
Update the image path in `k8s/deployment.yaml` with your built image.
Apply the deployment and service:
```bash
kubectl apply -f k8s/deployment.yaml
kubectl apply -f k8s/service.yaml
```

## 4. Playwright Container Considerations
The deployment is configured to run Playwright in **headless mode**.
If the container environment (like certain security contexts in Kubernetes) restricts browser execution, ensure that:
1. `HEADLESS=true` environment variable is set.
2. The image contains necessary shared libraries (included in the provided `Dockerfile` based on `mcr.microsoft.com/playwright`).
3. If running as non-root, you may need to pass `--no-sandbox` as a flag in the Go code when launching Playwright.

## 5. Rollback
In case of issues, you can roll back to a previous version using:
```bash
kubectl rollout undo deployment hr-sorter
```

## 6. Managing Secrets with CI/CD
In a real production environment, secrets should not be in `k8s/secret.yaml`.
Instead, use:
- **GitLab CI Variables** or **GitHub Secrets**.
- **External Secrets Operator** (e.g., HashiCorp Vault).
- **Environment specific overlays** (e.g., Kustomize or Helm).
