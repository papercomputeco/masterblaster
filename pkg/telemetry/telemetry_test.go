package telemetry_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/papercomputeco/masterblaster/pkg/telemetry"
)

var _ = Describe("Telemetry", func() {
	Describe("UUID persistence", func() {
		var (
			tmpDir  string
			oldPath string
		)

		BeforeEach(func() {
			tmpDir = GinkgoT().TempDir()
			statePath := filepath.Join(tmpDir, "telemetry.json")
			oldPath = telemetry.SetTelemetryFilePath(statePath)
		})

		AfterEach(func() {
			telemetry.SetTelemetryFilePath(oldPath)
		})

		It("creates a new state file on first run", func() {
			id, isFirst, err := telemetry.GetOrCreateUniqueID()
			Expect(err).NotTo(HaveOccurred())
			Expect(isFirst).To(BeTrue())
			Expect(id).NotTo(BeEmpty())
		})

		It("reuses existing UUID on subsequent runs", func() {
			id1, isFirst1, err := telemetry.GetOrCreateUniqueID()
			Expect(err).NotTo(HaveOccurred())
			Expect(isFirst1).To(BeTrue())

			id2, isFirst2, err := telemetry.GetOrCreateUniqueID()
			Expect(err).NotTo(HaveOccurred())
			Expect(isFirst2).To(BeFalse())
			Expect(id2).To(Equal(id1))
		})

		It("writes the state file with 0600 permissions", func() {
			_, _, err := telemetry.GetOrCreateUniqueID()
			Expect(err).NotTo(HaveOccurred())

			info, err := os.Stat(filepath.Join(tmpDir, "telemetry.json"))
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0600)))
		})

		It("stores valid JSON with expected fields", func() {
			_, _, err := telemetry.GetOrCreateUniqueID()
			Expect(err).NotTo(HaveOccurred())

			data, err := os.ReadFile(filepath.Join(tmpDir, "telemetry.json"))
			Expect(err).NotTo(HaveOccurred())

			var state telemetry.State
			Expect(json.Unmarshal(data, &state)).To(Succeed())
			Expect(state.ID).NotTo(BeEmpty())
			Expect(state.FirstRunDate).NotTo(BeEmpty())
		})

		It("regenerates UUID when state file contains invalid JSON", func() {
			statePath := filepath.Join(tmpDir, "telemetry.json")
			Expect(os.WriteFile(statePath, []byte("not json"), 0600)).To(Succeed())

			id, isFirst, err := telemetry.GetOrCreateUniqueID()
			Expect(err).NotTo(HaveOccurred())
			Expect(isFirst).To(BeTrue())
			Expect(id).NotTo(BeEmpty())
		})
	})

	Describe("IsCI", func() {
		It("returns true when CI is set", func() {
			GinkgoT().Setenv("CI", "true")
			Expect(telemetry.IsCI()).To(BeTrue())
		})

		It("returns true when GITHUB_ACTIONS is set", func() {
			GinkgoT().Setenv("GITHUB_ACTIONS", "true")
			Expect(telemetry.IsCI()).To(BeTrue())
		})

		It("returns false when no CI env vars are set", func() {
			for _, env := range []string{
				"CI", "GITHUB_ACTIONS", "GITLAB_CI", "CIRCLECI",
				"TRAVIS", "JENKINS_URL", "BUILDKITE", "CODEBUILD_BUILD_ID",
			} {
				GinkgoT().Setenv(env, "")
			}
			Expect(telemetry.IsCI()).To(BeFalse())
		})
	})

	Describe("Context", func() {
		It("round-trips a client through context", func() {
			ctx := context.Background()
			Expect(telemetry.FromContext(ctx)).To(BeNil())

			ctx = telemetry.WithContext(ctx, nil)
			Expect(telemetry.FromContext(ctx)).To(BeNil())
		})
	})

	Describe("PosthogClient nil safety", func() {
		It("does not panic when calling capture methods on nil client", func() {
			var client *telemetry.PosthogClient
			Expect(func() {
				client.CaptureInstall()
				client.CaptureCommandRun("test")
				client.CaptureUp("mixtape", true)
				client.CaptureDown(true)
				client.CaptureSSH()
				client.CapturePull("mixtape", true)
				client.CaptureError("test", "runtime")
			}).NotTo(Panic())
		})

		It("does not panic when calling Done on nil client", func() {
			var client *telemetry.PosthogClient
			Expect(func() {
				client.Done()
			}).NotTo(Panic())
		})
	})
})
