## Krane CLI

A lightweight CLI that discovers container images in your Kubernetes cluster and mirrors them to AWS ECR.

### Features
- Discovers pod and init container images; optionally shows owning resources (`--show-sources`)
- Registry-to-registry push to AWS ECR (`crane.Copy`)
  - Preserves multi-arch manifests; restrict to a single platform with `--platform os/arch`
- Automatically creates ECR repositories (no-op if they already exist)
- Checks if a target tag exists in ECR and skips (`--skip-existing`)
- Filters:
  - Namespaces: `--include-namespaces/--exclude-namespaces` (regex if compilable, otherwise prefix)
  - Image names: `--include/--exclude` (regex if compilable, otherwise prefix)
- Outputs (list): `table`, `json`, `yaml` (grouped view with `--show-sources`)
- Connects via kubeconfig (`~/.kube/config`)

---

### Requirements
- Kubernetes access: local `~/.kube/config` (uses the current context). You may point to it with `KUBECONFIG`.
- AWS credentials: a user/role authorized for ECR. The CLI uses the AWS SDK default credential chain; no extra flags are required.
  - Supported methods: `AWS_PROFILE`, `AWS_ACCESS_KEY_ID/SECRET_ACCESS_KEY`, SSO, instance/IRSA role, etc.
  - Region: `--region` flag or `AWS_REGION`/`AWS_DEFAULT_REGION`.

Example IAM policy:
```json
{
  "Version": "2012-10-17",
  "Statement": [
    { "Effect": "Allow", "Action": ["sts:GetCallerIdentity"], "Resource": "*" },
    {
      "Effect": "Allow",
      "Action": [
        "ecr:GetAuthorizationToken",
        "ecr:CreateRepository",
        "ecr:DescribeImages",
        "ecr:BatchCheckLayerAvailability",
        "ecr:InitiateLayerUpload",
        "ecr:UploadLayerPart",
        "ecr:CompleteLayerUpload",
        "ecr:PutImage",
        "ecr:BatchGetImage",
        "ecr:GetDownloadUrlForLayer"
      ],
      "Resource": "*"
    }
  ]
}
```

### Usage

#### List
```bash
krane list [--all-namespaces] [--namespace ns] \
  [--include PATTERN,...] [--exclude PATTERN,...] \
  [--include-namespaces NS,...] [--exclude-namespaces NS,...] \
  [--format table|json|yaml] [--show-sources]
```

Examples:
```bash
# Entire cluster
krane list --all-namespaces

# Only prod namespaces and nginx images
krane list --include-namespaces "^prod-" --include "nginx"

# Show owning resources (Deployment/Job/CronJob)
krane list --all-namespaces --show-sources --format table
```

#### Push (ECR)
```bash
krane push [--all-namespaces | --namespace ns] \
  --region REGION --prefix PREFIX [--dry-run] [--platform os/arch] \
  [--skip-existing] \
  [--include PATTERN,...] [--exclude PATTERN,...] \
  [--include-namespaces NS,...] [--exclude-namespaces NS,...]
```

Selected flags:
- `--region`: AWS region (e.g., `eu-west-1`)
- `--prefix`: prefix for ECR repository names (e.g., `k8s-backup`)
- `--platform`: copy a single platform (e.g., `linux/amd64`); if empty, multi-arch is preserved
- `--dry-run`: show what would be pushed without executing
- `--skip-existing`: skip mirroring when the target ECR tag already exists
- `--include/--exclude`: image-name filters (regex if compilable, otherwise prefix)
- `--include-namespaces/--exclude-namespaces`: namespace filters (regex/prefix)

Examples:
```bash
# Mirror all images to ECR (preserve multi-arch)
krane push --all-namespaces --region eu-west-1 --prefix k8s-backup

# Only linux/amd64 platform
krane push --region eu-west-1 --prefix k8s-backup --platform linux/amd64

# Prod namespaces and selected images (skipping existing tags)
krane push --include-namespaces "^prod-" --include "(nginx|busybox)" \
  --region eu-west-1 --prefix k8s-backup --skip-existing
```

### Troubleshooting
- AWS authentication errors: verify your profile/role and region
- Kubernetes access: check `KUBECONFIG` or `~/.kube/config`
