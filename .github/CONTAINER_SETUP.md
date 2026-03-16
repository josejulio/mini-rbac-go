# Container Image Publishing Setup

This repository is configured to automatically build and publish container images to Quay.io.

## Prerequisites

You need to configure the following GitHub repository secrets:

1. `QUAY_USERNAME` - Your Quay.io username (set to: `jmartine`)
2. `QUAY_PASSWORD` - Your Quay.io password or robot token

## Setting up GitHub Secrets

1. Go to your GitHub repository: `Settings` → `Secrets and variables` → `Actions`
2. Click `New repository secret`
3. Add both secrets:
   - Name: `QUAY_USERNAME`, Value: `jmartine`
   - Name: `QUAY_PASSWORD`, Value: `<your-quay-password-or-token>`

**Recommended:** Use a Quay.io robot account token instead of your password:
- Go to https://quay.io/organization/jmartine?tab=robots
- Create a new robot account with write permissions
- Use the robot token as `QUAY_PASSWORD`

## How it Works

The GitHub Actions workflow (`.github/workflows/publish-image.yml`) will:

1. **On push to main/master branch:**
   - Build the image for `linux/amd64` and `linux/arm64`
   - Tag as `quay.io/jmartine/mini-rbac-go:latest`
   - Tag as `quay.io/jmartine/mini-rbac-go:main-<git-sha>`

2. **On git tags (e.g., v1.0.0):**
   - Build multi-arch images
   - Tag as `quay.io/jmartine/mini-rbac-go:v1.0.0`
   - Tag as `quay.io/jmartine/mini-rbac-go:1.0`
   - Tag as `quay.io/jmartine/mini-rbac-go:1`
   - Tag as `quay.io/jmartine/mini-rbac-go:latest`

3. **Manual trigger:**
   - Can be triggered manually via GitHub Actions UI

## Using the Published Images

```bash
# Pull the latest version
docker pull quay.io/jmartine/mini-rbac-go:latest

# Pull a specific version
docker pull quay.io/jmartine/mini-rbac-go:v1.0.0

# Run the container
docker run -p 8080:8080 \
  -e DATABASE_HOST=localhost \
  -e DATABASE_NAME=rbac \
  quay.io/jmartine/mini-rbac-go:latest
```

## Local Testing

Test the Dockerfile locally before pushing:

### Using Makefile (Recommended)

```bash
# Build the image (uses your $USER as namespace)
make image-build

# Build with custom namespace/tag
make image-build IMAGE_NAMESPACE=myuser IMAGE_TAG=test

# Build and push to your registry
make image-build-push IMAGE_NAMESPACE=myuser

# Run it
docker run -p 8080:8080 \
  -e DATABASE_HOST=host.docker.internal \
  -e DATABASE_NAME=rbac \
  quay.io/myuser/mini-rbac-go:latest
```

### Manual Docker Commands

```bash
# Build the image
docker build -t mini-rbac-go:test .

# Run it
docker run -p 8080:8080 mini-rbac-go:test
```

**Note:** The Makefile automatically detects whether you have `docker` or `podman` installed.

## Creating a Release

To publish a versioned release:

```bash
# Create and push a version tag
git tag -a v1.0.0 -m "Release version 1.0.0"
git push origin v1.0.0
```

The GitHub Actions workflow will automatically build and publish the tagged version.
