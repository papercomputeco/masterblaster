package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"dagger/masterblaster/internal/dagger"
)

type Masterblaster struct {
	// Project source directory
	//
	// +private
	Source *dagger.Directory
}

// New creates a new masterblaster dagger module instance
func New(
	// Project source directory.
	//
	// +defaultPath="/"
	// +ignore=[".git", ".direnv", "build", "tmp"]
	source *dagger.Directory,
) *Masterblaster {
	return &Masterblaster{
		Source: source,
	}
}

type buildTarget struct {
	goos   string
	goarch string
}

// goContainer returns a base Go container with the project source mounted.
func (m *Masterblaster) goContainer() *dagger.Container {
	return dag.Container().
		From("golang:1.25-bookworm").
		WithMountedCache("/go/pkg/mod", dag.CacheVolume("go-mod")).
		WithMountedCache("/root/.cache/go-build", dag.CacheVolume("go-build")).
		WithDirectory("/src", m.Source).
		WithWorkdir("/src")
}

// Build cross-compiles the mb binary for all supported Linux platforms
// and returns a directory containing the output binaries organized
// as {os}/{arch}/mb.
//
// Darwin builds are excluded because the vz (Apple Virtualization.framework)
// dependency requires cgo with Objective-C and cannot be cross-compiled from
// Linux. Darwin artifacts are built on native Apple hardware via GitHub Actions.
func (m *Masterblaster) Build(
	ctx context.Context,

	// Linker flags for go build
	// +optional
	// +default="-s -w"
	ldflags string,
) *dagger.Directory {
	targets := []buildTarget{
		{"linux", "amd64"},
		{"linux", "arm64"},
	}

	golang := m.goContainer()
	outputs := dag.Directory()

	for _, target := range targets {
		path := fmt.Sprintf("%s/%s/", target.goos, target.goarch)

		build := golang.
			WithEnvVariable("CGO_ENABLED", "0").
			WithEnvVariable("GOOS", target.goos).
			WithEnvVariable("GOARCH", target.goarch).
			WithExec([]string{"go", "build", "-ldflags", ldflags, "-o", path + "mb", "."})

		outputs = outputs.WithDirectory(path, build.Directory(path))
	}

	return outputs
}

// BuildRelease compiles versioned release binaries with embedded version info
// and generates SHA256 checksums for each artifact.
func (m *Masterblaster) BuildRelease(
	ctx context.Context,

	// Version string of build
	version string,

	// Git commit SHA of build
	commit string,
) *dagger.Directory {
	buildtime := time.Now()

	ldflags := []string{
		"-s",
		"-w",
		fmt.Sprintf("-X 'github.com/papercomputeco/masterblaster/pkg/utils.Version=%s'", version),
		fmt.Sprintf("-X 'github.com/papercomputeco/masterblaster/pkg/utils.Sha=%s'", commit),
		fmt.Sprintf("-X 'github.com/papercomputeco/masterblaster/pkg/utils.Buildtime=%s'", buildtime),
	}

	dir := m.Build(ctx, strings.Join(ldflags, " "))
	return dag.Checksumer().Checksum(dir)
}
