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
) (*dagger.Directory, error) {
	artifacts := m.BuildRelease(ctx, version, commit)

	uploader := dag.Bucketuploader(endpoint, bucket, accessKeyId, secretAccessKey)
	if err := uploader.UploadLatest(ctx, artifacts, version); err != nil {
		return nil, fmt.Errorf("failed to upload latest release: %w", err)
	}

	return artifacts, nil
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
) (*dagger.Directory, error) {
	artifacts := m.BuildRelease(ctx, "nightly", commit)

	uploader := dag.Bucketuploader(endpoint, bucket, accessKeyId, secretAccessKey)
	if err := uploader.UploadNightly(ctx, artifacts); err != nil {
		return nil, fmt.Errorf("failed to upload nightly release: %w", err)
	}

	return artifacts, nil
}

// UploadDarwinArtifacts uploads the pre-built darwin/arm64 mb binary and its
// checksum to the S3 bucket. Each file is uploaded individually via UploadFile
// with the appropriate prefix (e.g., "v1.0.0/darwin/arm64", "latest/darwin/arm64",
// or "nightly/darwin/arm64").
//
// This is called from GitHub Actions after the native macOS build to place
// darwin artifacts alongside the Linux artifacts already uploaded by Dagger.
func (m *Masterblaster) UploadDarwinArtifacts(
	ctx context.Context,

	// The darwin/arm64 mb binary
	binary *dagger.File,

	// The darwin/arm64 mb.sha256 checksum file
	checksum *dagger.File,

	// Bucket path prefixes to upload under (e.g., ["v1.0.0/darwin/arm64", "latest/darwin/arm64"])
	prefixes []string,

	// Bucket endpoint URL
	endpoint *dagger.Secret,

	// Bucket name
	bucket *dagger.Secret,

	// Bucket access key ID
	accessKeyId *dagger.Secret,

	// Bucket secret access key
	secretAccessKey *dagger.Secret,
) error {
	uploader := dag.Bucketuploader(endpoint, bucket, accessKeyId, secretAccessKey)

	for _, prefix := range prefixes {
		if err := uploader.UploadFile(ctx, binary, dagger.BucketuploaderUploadFileOpts{Prefix: prefix}); err != nil {
			return fmt.Errorf("failed to upload mb to %s: %w", prefix, err)
		}
		if err := uploader.UploadFile(ctx, checksum, dagger.BucketuploaderUploadFileOpts{Prefix: prefix}); err != nil {
			return fmt.Errorf("failed to upload mb.sha256 to %s: %w", prefix, err)
		}
	}

	return nil
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
