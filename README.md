## Krane CLI

A lightweight CLI that discovers container images in your Kubernetes cluster and mirrors them to AWS ECR.

### Features
- Discovers pod and init container images; optionally shows owning resources (`--show-sources`)
- Registry-to-registry push to AWS ECR (`crane.Copy`)
  - Preserves multi-arch manifests; restrict to a single platform with `--platform os/arch`
  - Parallel image processing with configurable concurrency (`--max-concurrent`)
- Automatically creates ECR repositories (no-op if they already exist)
- Checks if a target tag exists in ECR and skips (`--skip-existing`)
- Filters:
  - Namespaces: `--include-namespaces/--exclude-namespaces` (regex if compilable, otherwise prefix)
  - Image names: `--include/--exclude` (regex if compilable, otherwise prefix)
- Outputs (list): `table`, `json`, `yaml` (grouped view with `--show-sources`)

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
krane list [-A|--all-namespaces] [-n|--namespace ns] \
  [-i|--include PATTERN,...] [-e|--exclude PATTERN,...] \
  [--include-namespaces NS,...] [--exclude-namespaces NS,...] \
  [-o|--format table|json|yaml] [-s|--show-sources]
```

Examples:
```bash
# Entire cluster
krane list -A

# Only prod namespaces and nginx images
krane list --include-namespaces "^prod-" -i "nginx"

# Show owning resources (Deployment/Job/CronJob)
krane list -A -s -o table

# Specific namespace with JSON output
krane list -n kube-system -o json

# Exclude system images
krane list -A -e "k8s.gcr.io" -e "registry.k8s.io"
```

#### Push (ECR)
```bash
krane push [-A|--all-namespaces | -n|--namespace ns] \
  [-r|--region REGION] [--prefix PREFIX] [-d|--dry-run] [-p|--platform os/arch] \
  [-S|--skip-existing] [-c|--max-concurrent N] \
  [-i|--include PATTERN,...] [-e|--exclude PATTERN,...] \
  [--include-namespaces NS,...] [--exclude-namespaces NS,...]
```

Selected flags:
- `-r|--region`: AWS region (e.g., `eu-west-1`)
- `--prefix`: prefix for ECR repository names (default: `krane`)
- `-p|--platform`: copy a single platform (e.g., `linux/amd64`); if empty, multi-arch is preserved
- `-d|--dry-run`: show what would be pushed without executing
- `-S|--skip-existing`: skip mirroring when the target ECR tag already exists
- `-i|--include` / `-e|--exclude`: image-name filters (regex if compilable, otherwise prefix)
- `--include-namespaces/--exclude-namespaces`: namespace filters (regex/prefix)
- `-c|--max-concurrent`: number of concurrent image transfers (default: 3)

Examples:
```bash
# Mirror all images to ECR (preserve multi-arch)
krane push -A -r eu-west-1

# Only linux/amd64 platform with custom prefix
krane push -r eu-west-1 --prefix k8s-backup -p linux/amd64

# Prod namespaces and selected images (skipping existing tags, 5 concurrent workers)
krane push --include-namespaces "^prod-" -i "(nginx|busybox)" \
  -r eu-west-1 -S -c 5

# Dry run for specific namespace  
krane push -n production -r us-east-1 -d

# Exclude system images from all namespaces
krane push -A -r eu-west-1 -e "k8s.gcr.io" -e "registry.k8s.io"
```

### Troubleshooting
- AWS authentication errors: verify your profile/role and region
- Kubernetes access: check `KUBECONFIG` or `~/.kube/config`
