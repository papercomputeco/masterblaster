package config

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Config", func() {

	Describe("Load", func() {

		Context("with a minimal config", func() {
			var cfg *JcardConfig

			BeforeEach(func() {
				dir := GinkgoT().TempDir()
				tomlContent := `
mixtape = "base"
`
				cfgPath := filepath.Join(dir, "jcard.toml")
				Expect(os.WriteFile(cfgPath, []byte(tomlContent), 0644)).To(Succeed())

				var err error
				cfg, err = Load(cfgPath)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should parse the mixtape field", func() {
				Expect(cfg.Mixtape).To(Equal("base"))
			})

			It("should apply default CPUs", func() {
				Expect(cfg.Resources.CPUs).To(Equal(2))
			})

			It("should apply default memory", func() {
				Expect(cfg.Resources.Memory).To(Equal("4GiB"))
			})

			It("should apply default disk", func() {
				Expect(cfg.Resources.Disk).To(Equal("20GiB"))
			})

			It("should apply default network mode", func() {
				Expect(cfg.Network.Mode).To(Equal("nat"))
			})

			It("should apply default agent restart policy", func() {
				Expect(cfg.Agent.Restart).To(Equal("no"))
			})

			It("should apply default agent grace period", func() {
				Expect(cfg.Agent.GracePeriod).To(Equal("30s"))
			})

			It("should apply default agent workdir", func() {
				Expect(cfg.Agent.Workdir).To(Equal("/workspace"))
			})
		})

		Context("with a full config", func() {
			var (
				cfg *JcardConfig
				dir string
			)

			BeforeEach(func() {
				dir = GinkgoT().TempDir()

				// Create a prompt file
				promptPath := filepath.Join(dir, "prompt.md")
				Expect(os.WriteFile(promptPath, []byte("fix the tests"), 0644)).To(Succeed())

				tomlContent := `
mixtape = "openclaw"
name = "my-agent"

[resources]
cpus = 8
memory = "16GiB"
disk = "100GiB"

[network]
mode = "nat"
forwards = [
    { host = 8080, guest = 8080, proto = "tcp" },
    { host = 9090, guest = 9090, proto = "udp" },
]
egress_allow = ["api.anthropic.com"]

[[shared]]
host = "./"
guest = "/workspace"
readonly = false

[[shared]]
host = "/tmp/data"
guest = "/data"
readonly = true

[secrets]
MY_SECRET = "secret-value"

[agent]
harness = "claude-code"
prompt_file = "./prompt.md"
workdir = "/workspace"
restart = "on-failure"
max_restarts = 5
timeout = "2h"
grace_period = "60s"
session = "my-session"

[agent.env]
FOO = "bar"
`
				cfgPath := filepath.Join(dir, "jcard.toml")
				Expect(os.WriteFile(cfgPath, []byte(tomlContent), 0644)).To(Succeed())

				var err error
				cfg, err = Load(cfgPath)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should parse the mixtape field", func() {
				Expect(cfg.Mixtape).To(Equal("openclaw"))
			})

			It("should parse the name field", func() {
				Expect(cfg.Name).To(Equal("my-agent"))
			})

			It("should parse resources", func() {
				Expect(cfg.Resources.CPUs).To(Equal(8))
				Expect(cfg.Resources.Memory).To(Equal("16GiB"))
				Expect(cfg.Resources.Disk).To(Equal("100GiB"))
			})

			It("should parse network forwards", func() {
				Expect(cfg.Network.Forwards).To(HaveLen(2))
				Expect(cfg.Network.Forwards[0].Host).To(Equal(8080))
				Expect(cfg.Network.Forwards[1].Proto).To(Equal("udp"))
			})

			It("should parse egress allow list", func() {
				Expect(cfg.Network.EgressAllow).To(ConsistOf("api.anthropic.com"))
			})

			It("should parse shared mounts", func() {
				Expect(cfg.Shared).To(HaveLen(2))
				Expect(cfg.Shared[0].Guest).To(Equal("/workspace"))
				Expect(cfg.Shared[1].ReadOnly).To(BeTrue())
			})

			It("should parse secrets", func() {
				Expect(cfg.Secrets).To(HaveKeyWithValue("MY_SECRET", "secret-value"))
			})

			It("should parse agent configuration", func() {
				Expect(cfg.Agent.Harness).To(Equal("claude-code"))
				Expect(cfg.Agent.PromptFile).To(Equal(filepath.Join(dir, "prompt.md")))
				Expect(cfg.Agent.Restart).To(Equal("on-failure"))
				Expect(cfg.Agent.MaxRestarts).To(Equal(5))
				Expect(cfg.Agent.Timeout).To(Equal("2h"))
				Expect(cfg.Agent.GracePeriod).To(Equal("60s"))
				Expect(cfg.Agent.Session).To(Equal("my-session"))
			})

			It("should parse agent env", func() {
				Expect(cfg.Agent.Env).To(HaveKeyWithValue("FOO", "bar"))
			})
		})
	})

	Describe("Validation", func() {

		DescribeTable("should reject invalid configs",
			func(toml string) {
				dir := GinkgoT().TempDir()
				cfgPath := filepath.Join(dir, "jcard.toml")
				Expect(os.WriteFile(cfgPath, []byte(toml), 0644)).To(Succeed())

				_, err := Load(cfgPath)
				Expect(err).To(HaveOccurred())
			},
			Entry("invalid network mode", `
mixtape = "base"

[network]
mode = "invalid"
`),
			Entry("invalid harness", `
mixtape = "base"

[agent]
harness = "invalid-harness"
`),
			Entry("invalid restart policy", `
mixtape = "base"

[agent]
restart = "invalid"
`),
			Entry("invalid port forward (host port 0)", `
mixtape = "base"

[network]
mode = "nat"
forwards = [
    { host = 0, guest = 80, proto = "tcp" },
]
`),
		)
	})

	Describe("Mixtape name:tag format", func() {
		It("should accept name:tag format", func() {
			dir := GinkgoT().TempDir()
			tomlContent := `mixtape = "opencode-mixtape:0.1.0"`
			cfgPath := filepath.Join(dir, "jcard.toml")
			Expect(os.WriteFile(cfgPath, []byte(tomlContent), 0644)).To(Succeed())

			cfg, err := Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Mixtape).To(Equal("opencode-mixtape:0.1.0"))
		})

		It("should accept bare name without tag", func() {
			dir := GinkgoT().TempDir()
			tomlContent := `mixtape = "base"`
			cfgPath := filepath.Join(dir, "jcard.toml")
			Expect(os.WriteFile(cfgPath, []byte(tomlContent), 0644)).To(Succeed())

			cfg, err := Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Mixtape).To(Equal("base"))
		})

		It("should default to base:latest when mixtape is omitted", func() {
			dir := GinkgoT().TempDir()
			tomlContent := `
[resources]
cpus = 2
`
			cfgPath := filepath.Join(dir, "jcard.toml")
			Expect(os.WriteFile(cfgPath, []byte(tomlContent), 0644)).To(Succeed())

			cfg, err := Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Mixtape).To(Equal("base:latest"))
		})
	})

	Describe("Name defaulting", func() {
		It("should default the name to the directory name", func() {
			dir := GinkgoT().TempDir()
			tomlContent := `mixtape = "base"`
			cfgPath := filepath.Join(dir, "jcard.toml")
			Expect(os.WriteFile(cfgPath, []byte(tomlContent), 0644)).To(Succeed())

			cfg, err := Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Name).To(Equal(filepath.Base(dir)))
		})
	})

	Describe("Environment variable expansion", func() {
		It("should expand env vars in secrets", func() {
			Expect(os.Setenv("MB_TEST_KEY", "test-api-key-123")).To(Succeed())
			DeferCleanup(os.Unsetenv, "MB_TEST_KEY")

			dir := GinkgoT().TempDir()
			tomlContent := `
mixtape = "base"

[secrets]
API_KEY = "${MB_TEST_KEY}"
`
			cfgPath := filepath.Join(dir, "jcard.toml")
			Expect(os.WriteFile(cfgPath, []byte(tomlContent), 0644)).To(Succeed())

			cfg, err := Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Secrets).To(HaveKeyWithValue("API_KEY", "test-api-key-123"))
		})
	})

	Describe("DefaultJcardTOML", func() {
		It("should return non-empty valid TOML", func() {
			content := DefaultJcardTOML()
			Expect(content).NotTo(BeEmpty())

			dir := GinkgoT().TempDir()
			cfgPath := filepath.Join(dir, "jcard.toml")
			Expect(os.WriteFile(cfgPath, []byte(content), 0644)).To(Succeed())

			_, err := Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Workdir defaulting", func() {
		It("should default workdir to the first shared mount guest path", func() {
			dir := GinkgoT().TempDir()
			tomlContent := `
mixtape = "base"

[[shared]]
host = "./"
guest = "/code"
`
			cfgPath := filepath.Join(dir, "jcard.toml")
			Expect(os.WriteFile(cfgPath, []byte(tomlContent), 0644)).To(Succeed())

			cfg, err := Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Agent.Workdir).To(Equal("/code"))
		})
	})
})
