package vm

import (
	"bytes"
	"fmt"
	"os"
	"text/template"

	diskfs "github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/filesystem"
	"github.com/diskfs/go-diskfs/filesystem/iso9660"
)

const metaDataTemplate = `instance-id: {{ .InstanceID }}
local-hostname: {{ .Hostname }}
`

const userDataTemplate = `#cloud-config
users:
  - name: {{ .User }}
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    ssh_authorized_keys:
      - {{ .SSHPublicKey }}

package_update: true
package_upgrade: false

packages:
{{- range .Packages }}
  - {{ . }}
{{- end }}

runcmd:
  # Install OpenCode from the cloud-init ISO
  - mkdir -p /mnt/cidata
  - mount -t iso9660 -o ro /dev/disk/by-label/cidata /mnt/cidata || true
  - if [ -f /mnt/cidata/opencode ]; then cp /mnt/cidata/opencode /usr/local/bin/opencode && chmod 755 /usr/local/bin/opencode; fi
  - umount /mnt/cidata 2>/dev/null || true
{{- if .ConfigJSON }}
  # Write OpenCode config
  - mkdir -p /home/{{ .User }}/.config/opencode
  - |
    cat > /home/{{ .User }}/.config/opencode/config.json << 'OPENCODE_EOF'
    {{ .ConfigJSON }}
    OPENCODE_EOF
  - chown -R {{ .User }}:{{ .User }} /home/{{ .User }}/.config/opencode
{{- end }}
{{- range $key, $val := .Environment }}
  - echo 'export {{ $key }}="{{ $val }}"' >> /home/{{ $.User }}/.bashrc
{{- end }}
{{- range $tag, $vol := .Volumes }}
  - mkdir -p {{ $vol.Guest }}
  - mount -t 9p -o trans=virtio,version=9p2000.L {{ $tag }} {{ $vol.Guest }}
  - echo '{{ $tag }} {{ $vol.Guest }} 9p trans=virtio,version=9p2000.L 0 0' >> /etc/fstab
{{- end }}
  # Signal that provisioning is complete
  - touch /var/lib/cloud/instance/mb-ready

write_files:
  - path: /etc/motd
    content: |
      ╔══════════════════════════════════════╗
      ║   Masterblaster Agent Sandbox        ║
      ║   Run 'opencode' to start agent      ║
      ╚══════════════════════════════════════╝

final_message: "Masterblaster provisioning complete after $UPTIME seconds"
`

// cloudInitTemplateData bundles all template data for cloud-init rendering.
type cloudInitTemplateData struct {
	CloudInitData
	Volumes map[string]volumeMount
}

// volumeMount is a local copy to avoid import cycle in templates.
type volumeMount struct {
	Host  string
	Guest string
}

// generateCloudInitISO renders cloud-init meta-data and user-data templates,
// then writes them to a NoCloud ISO9660 image at isoPath.
// If data.OpenCodeBinary is non-nil, the binary is embedded on the ISO.
func generateCloudInitISO(isoPath string, data CloudInitData, volumes map[string]string) error {
	// Render meta-data
	metaData, err := renderTemplate("meta-data", metaDataTemplate, data)
	if err != nil {
		return fmt.Errorf("rendering meta-data: %w", err)
	}

	// Build volume map for template
	volMap := make(map[string]volumeMount, len(volumes))
	for tag, guest := range volumes {
		volMap[tag] = volumeMount{Guest: guest}
	}

	tmplData := cloudInitTemplateData{
		CloudInitData: data,
		Volumes:       volMap,
	}

	// Render user-data
	userData, err := renderTemplate("user-data", userDataTemplate, tmplData)
	if err != nil {
		return fmt.Errorf("rendering user-data: %w", err)
	}

	// Generate ISO using go-diskfs
	if err := writeNoCloudISO(isoPath, metaData, userData, data.OpenCodeBinary); err != nil {
		return fmt.Errorf("writing cloud-init ISO: %w", err)
	}

	return nil
}

func renderTemplate(name, tmpl string, data interface{}) ([]byte, error) {
	t, err := template.New(name).Parse(tmpl)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeNoCloudISO creates a minimal ISO9660 filesystem with the cidata volume
// label containing meta-data, user-data, and optionally the OpenCode binary.
func writeNoCloudISO(isoPath string, metaData, userData, openCodeBin []byte) error {
	// Size the ISO to fit the OpenCode binary (~50MB) plus headroom,
	// or use a small default if no binary is embedded.
	isoSize := int64(10 * 1024 * 1024) // 10MB default
	if len(openCodeBin) > 0 {
		// Binary size + 10MB headroom, rounded up to nearest MB
		isoSize = int64(len(openCodeBin)) + 10*1024*1024
	}

	d, err := diskfs.Create(isoPath, isoSize, diskfs.SectorSize(2048))
	if err != nil {
		return fmt.Errorf("creating disk image: %w", err)
	}

	fspec := disk.FilesystemSpec{
		Partition:   0,
		FSType:      filesystem.TypeISO9660,
		VolumeLabel: "cidata",
	}
	fs, err := d.CreateFilesystem(fspec)
	if err != nil {
		return fmt.Errorf("creating ISO filesystem: %w", err)
	}

	// Write meta-data
	if err := writeFileToFS(fs, "/meta-data", metaData); err != nil {
		return fmt.Errorf("writing meta-data to ISO: %w", err)
	}

	// Write user-data
	if err := writeFileToFS(fs, "/user-data", userData); err != nil {
		return fmt.Errorf("writing user-data to ISO: %w", err)
	}

	// Embed the OpenCode binary if provided
	if len(openCodeBin) > 0 {
		if err := writeFileToFS(fs, "/opencode", openCodeBin); err != nil {
			return fmt.Errorf("writing OpenCode binary to ISO: %w", err)
		}
	}

	// Finalize the ISO
	iso, ok := fs.(*iso9660.FileSystem)
	if !ok {
		return fmt.Errorf("unexpected filesystem type")
	}
	if err := iso.Finalize(iso9660.FinalizeOptions{
		RockRidge:        true,
		VolumeIdentifier: "cidata",
	}); err != nil {
		return fmt.Errorf("finalizing ISO: %w", err)
	}

	return nil
}

func writeFileToFS(fs filesystem.FileSystem, path string, content []byte) error {
	f, err := fs.OpenFile(path, os.O_CREATE|os.O_RDWR)
	if err != nil {
		return err
	}
	_, err = f.Write(content)
	return err
}
