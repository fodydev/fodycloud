# Deploy from CI

FodyCloud doesn't redeploy automatically when you push. This guide shows how to deploy on every
push to your main branch from GitHub Actions or GitLab CI, and fail the pipeline when the
deployment fails.

**You need:**

- an app whose stack is already confirmed (deploy it once from the dashboard first);
- an API token with **Allow changes** ticked, used only by this pipeline (see
  [Authentication](authentication.md)).

## How it works

The pipeline calls two endpoints:

1. `POST /apps/{name}/deploy` queues a build of the branch's latest commit and returns the
   deployment.
2. `GET /apps/{name}/deployments/{id}` is polled until the status is `running` (success),
   `failed` or `cancelled`.

## The deploy script

Save this script in your repository as `scripts/fodycloud-deploy.sh`. Both CI examples below
use it.

```bash
#!/usr/bin/env bash
# Deploys a FodyCloud app and waits for the result.
# Needs: FC_TOKEN (secret), FC_APP; optional FC_API.
set -euo pipefail
API="${FC_API:-https://api.fodycloud.example/v1}"
auth=(-H "Authorization: Bearer $FC_TOKEN")

resp="$(curl -sS --fail-with-body -X POST "${auth[@]}" "$API/apps/$FC_APP/deploy")"
id="$(jq -r .id <<<"$resp")"
echo "Deployment $id queued"

for _ in $(seq 1 240); do                       # up to 20 minutes
  dep="$(curl -sS --fail-with-body "${auth[@]}" "$API/apps/$FC_APP/deployments/$id")"
  status="$(jq -r .status <<<"$dep")"
  case "$status" in
    running)
      echo "Deployed commit $(jq -r .commit_sha <<<"$dep")"
      exit 0 ;;
    failed|cancelled)
      echo "Deployment $status: $(jq -r '.error // ""' <<<"$dep")"
      curl -sS "${auth[@]}" "$API/apps/$FC_APP/deployments/$id/log" | jq -r .log | tail -n 40
      exit 1 ;;
  esac
  sleep 5
done
echo "Timed out waiting for deployment $id"; exit 1
```

The script prints the last 40 lines of the build log when the deployment fails, so the reason
appears in your CI output.

## GitHub Actions

1. In your repository, open **Settings → Secrets and variables → Actions** and add a secret
   named `FC_TOKEN` with your token.
2. Add `.github/workflows/deploy.yml`:

```yaml
name: Deploy to FodyCloud
on:
  push:
    branches: [main]
concurrency: fodycloud-deploy        # one deployment at a time
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Deploy
        env:
          FC_TOKEN: ${{ secrets.FC_TOKEN }}
          FC_APP: my-portfolio
        run: bash scripts/fodycloud-deploy.sh
```

Replace `my-portfolio` with your app's name. To run tests first, add them as earlier steps or a
separate job that `deploy` needs.

## GitLab CI

1. In your project, open **Settings → CI/CD → Variables** and add `FC_TOKEN` with your token.
   Tick **Mask variable** and **Protect variable**.
2. Add a job to `.gitlab-ci.yml`:

```yaml
deploy:
  stage: deploy
  image: alpine:3.20
  resource_group: fodycloud          # one deployment at a time
  rules:
    - if: $CI_COMMIT_BRANCH == "main"
  variables:
    FC_APP: my-portfolio
  before_script:
    - apk add --no-cache bash curl jq
  script:
    - bash scripts/fodycloud-deploy.sh
```

## Good practices

- **Deploy only from the branch FodyCloud builds.** FodyCloud builds the app's configured
  branch, not the commit that triggered the pipeline. Deploying from another branch would
  rebuild the configured branch.
- **One deployment at a time.** A new deploy request cancels one that is still queued; the
  `concurrency` and `resource_group` settings above avoid confusing results.
- **Keep the 20 builds per hour limit in mind** if you push often: a `403` with
  `too many builds` means you reached it.
- **Use a dedicated token** and revoke it if the repository's secrets may have leaked.
