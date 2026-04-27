package mixtapes

import (
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
)

// digest produces a v1.Hash from a short identifier so tests stay readable.
func digest(hex string) v1.Hash {
	// Pad to 64 hex chars so v1.Hash accepts it.
	const zeros = "0000000000000000000000000000000000000000000000000000000000000000"
	return v1.Hash{Algorithm: "sha256", Hex: (hex + zeros)[:64]}
}

// manifest constructs an OCI descriptor with the given arch (or no platform
// if arch is "").
func manifest(d v1.Hash, goos, arch string) v1.Descriptor {
	m := v1.Descriptor{Digest: d}
	if arch != "" {
		m.Platform = &v1.Platform{OS: goos, Architecture: arch}
	}
	return m
}

func TestSelectIndexManifest(t *testing.T) {
	armRaw := digest("aa1")
	amdRaw := digest("bb2")
	amdQcow := digest("cc3")
	legacyRaw := digest("dd4") // no platform set
	legacyQcow := digest("ee5")

	// `raws` encodes which digests our fake `hasRaw` says are raw disks.
	type args struct {
		manifests []v1.Descriptor
		goos      string
		goarch    string
		raws      map[v1.Hash]bool
	}

	cases := []struct {
		name string
		args args
		want v1.Hash
	}{
		{
			name: "multi-arch index: host amd64 picks amd64 raw",
			args: args{
				manifests: []v1.Descriptor{
					manifest(armRaw, "linux", "arm64"),
					manifest(amdRaw, "linux", "amd64"),
					manifest(amdQcow, "linux", "amd64"),
				},
				goos:   "linux",
				goarch: "amd64",
				raws:   map[v1.Hash]bool{armRaw: true, amdRaw: true},
			},
			want: amdRaw,
		},
		{
			name: "multi-arch index: host arm64 picks arm64 raw",
			args: args{
				manifests: []v1.Descriptor{
					manifest(armRaw, "linux", "arm64"),
					manifest(amdRaw, "linux", "amd64"),
				},
				goos:   "linux",
				goarch: "arm64",
				raws:   map[v1.Hash]bool{armRaw: true, amdRaw: true},
			},
			want: armRaw,
		},
		{
			name: "platform match but no raw for host — uses qcow2",
			args: args{
				manifests: []v1.Descriptor{
					manifest(armRaw, "linux", "arm64"),
					manifest(amdQcow, "linux", "amd64"),
				},
				goos:   "linux",
				goarch: "amd64",
				raws:   map[v1.Hash]bool{armRaw: true},
			},
			want: amdQcow,
		},
		{
			name: "legacy single-arch index without Platform — uses raw fallback",
			args: args{
				manifests: []v1.Descriptor{
					manifest(legacyRaw, "", ""),
					manifest(legacyQcow, "", ""),
				},
				goos:   "linux",
				goarch: "amd64",
				raws:   map[v1.Hash]bool{legacyRaw: true},
			},
			want: legacyRaw,
		},
		{
			name: "no platform match, no raw — uses first manifest",
			args: args{
				manifests: []v1.Descriptor{
					manifest(armRaw, "linux", "arm64"),
				},
				goos:   "linux",
				goarch: "amd64",
				raws:   map[v1.Hash]bool{},
			},
			want: armRaw,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hasRaw := func(h v1.Hash) (bool, error) {
				return tc.args.raws[h], nil
			}
			got, err := selectIndexManifest(tc.args.manifests, tc.args.goos, tc.args.goarch, hasRaw)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestSelectIndexManifestEmpty(t *testing.T) {
	_, err := selectIndexManifest(nil, "linux", "amd64", func(v1.Hash) (bool, error) { return false, nil })
	if err == nil {
		t.Error("expected error for empty manifest list")
	}
}
