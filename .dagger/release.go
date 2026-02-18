package main

import (
	"context"
	"fmt"

	"dagger/masterblaster/internal/dagger"
)

// ReleaseLatest builds versioned release binaries and uploads them to the
// bucket under both the version prefix and a "latest" prefix.
func (m *Masterblaster) ReleaseLatest(
	ctx context.Context,

	// Version string (e.g., "v1.0.0")
	version string,

	// Git commit SHA
	commit string,

	// Bucket endpoint URL
	endpoint *dagger.Secret,

	// Bucket name
	bucket *dagger.Secret,

	// Bucket access key ID
	accessKeyId *dagger.Secret,

	// Bucket secret access key
	secretAccessKey *dagger.Secret,
) (string, error) {
	artifacts := m.BuildRelease(ctx, version, commit)

	uploader := dag.Bucketuploader(endpoint, bucket, accessKeyId, secretAccessKey)
	if err := uploader.UploadLatest(ctx, artifacts, version); err != nil {
		return "", fmt.Errorf("failed to upload latest release: %w", err)
	}

	return fmt.Sprintf("released %s and updated latest", version), nil
}

// ReleaseNightly builds nightly release binaries and uploads them to the
// bucket under the "nightly" prefix.
func (m *Masterblaster) ReleaseNightly(
	ctx context.Context,

	// Git commit SHA
	commit string,

	// Bucket endpoint URL
	endpoint *dagger.Secret,

	// Bucket name
	bucket *dagger.Secret,

	// Bucket access key ID
	accessKeyId *dagger.Secret,

	// Bucket secret access key
	secretAccessKey *dagger.Secret,
) (string, error) {
	artifacts := m.BuildRelease(ctx, "nightly", commit)

	uploader := dag.Bucketuploader(endpoint, bucket, accessKeyId, secretAccessKey)
	if err := uploader.UploadNightly(ctx, artifacts); err != nil {
		return "", fmt.Errorf("failed to upload nightly release: %w", err)
	}

	return "released nightly", nil
}

// UploadInstallScript uploads the install.sh script to the root of the bucket.
func (m *Masterblaster) UploadInstallSh(
	ctx context.Context,

	// Bucket endpoint URL
	endpoint *dagger.Secret,

	// Bucket name
	bucket *dagger.Secret,

	// Bucket access key ID
	accessKeyId *dagger.Secret,

	// Bucket secret access key
	secretAccessKey *dagger.Secret,
) (string, error) {
	installScript := m.Source.File("install.sh").WithName("install")

	uploader := dag.Bucketuploader(endpoint, bucket, accessKeyId, secretAccessKey)
	if err := uploader.UploadFile(ctx, installScript); err != nil {
		return "", fmt.Errorf("failed to upload install script: %w", err)
	}

	return "uploaded install.sh to bucket root", nil
}
