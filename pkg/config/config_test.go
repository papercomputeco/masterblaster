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

			It("should have an empty agents list", func() {
				Expect(cfg.Agents).To(BeEmpty())
			})
		})

		Context("with a single agent config", func() {
			var cfg *JcardConfig

			BeforeEach(func() {
				dir := GinkgoT().TempDir()
				tomlContent := `
mixtape = "base"

[[agents]]
harness = "claude-code"
`
				cfgPath := filepath.Join(dir, "jcard.toml")
				Expect(os.WriteFile(cfgPath, []byte(tomlContent), 0644)).To(Succeed())

				var err error
				cfg, err = Load(cfgPath)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should have one agent", func() {
				Expect(cfg.Agents).To(HaveLen(1))
			})

			It("should apply default agent restart policy", func() {
				Expect(cfg.Agents[0].Restart).To(Equal("no"))
			})

			It("should apply default agent grace period", func() {
				Expect(cfg.Agents[0].GracePeriod).To(Equal("30s"))
			})

			It("should apply default agent workdir", func() {
				Expect(cfg.Agents[0].Workdir).To(Equal("/workspace"))
			})

			It("should auto-generate agent name from harness", func() {
				Expect(cfg.Agents[0].Name).To(Equal("claude-code"))
			})

			It("should apply default agent type as sandboxed", func() {
				Expect(cfg.Agents[0].Type).To(Equal(AgentTypeSandboxed))
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

[[agents]]
name = "reviewer"
harness = "claude-code"
prompt_file = "./prompt.md"
workdir = "/workspace"
restart = "on-failure"
max_restarts = 5
timeout = "2h"
grace_period = "60s"
session = "my-session"

[agents.env]
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
				Expect(cfg.Agents).To(HaveLen(1))
				a := cfg.Agents[0]
				Expect(a.Name).To(Equal("reviewer"))
				Expect(a.Harness).To(Equal("claude-code"))
				Expect(a.PromptFile).To(Equal(filepath.Join(dir, "prompt.md")))
				Expect(a.Restart).To(Equal("on-failure"))
				Expect(a.MaxRestarts).To(Equal(5))
				Expect(a.Timeout).To(Equal("2h"))
				Expect(a.GracePeriod).To(Equal("60s"))
				Expect(a.Session).To(Equal("my-session"))
			})

			It("should parse agent env", func() {
				Expect(cfg.Agents[0].Env).To(HaveKeyWithValue("FOO", "bar"))
			})
		})

		Context("with multiple agents", func() {
			var cfg *JcardConfig

			BeforeEach(func() {
				dir := GinkgoT().TempDir()
				tomlContent := `
mixtape = "base"

[[agents]]
name = "reviewer"
harness = "claude-code"
prompt = "review the code"

[[agents]]
name = "coder"
harness = "opencode"
prompt = "implement the feature"

[[agents]]
harness = "gemini-cli"
prompt = "check for security issues"
`
				cfgPath := filepath.Join(dir, "jcard.toml")
				Expect(os.WriteFile(cfgPath, []byte(tomlContent), 0644)).To(Succeed())

				var err error
				cfg, err = Load(cfgPath)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should parse all agents", func() {
				Expect(cfg.Agents).To(HaveLen(3))
			})

			It("should preserve explicit names", func() {
				Expect(cfg.Agents[0].Name).To(Equal("reviewer"))
				Expect(cfg.Agents[1].Name).To(Equal("coder"))
			})

			It("should auto-generate name for unnamed agent", func() {
				Expect(cfg.Agents[2].Name).To(Equal("gemini-cli"))
			})

			It("should parse each agent's harness", func() {
				Expect(cfg.Agents[0].Harness).To(Equal("claude-code"))
				Expect(cfg.Agents[1].Harness).To(Equal("opencode"))
				Expect(cfg.Agents[2].Harness).To(Equal("gemini-cli"))
			})
		})

		Context("with duplicate unnamed harnesses", func() {
			var cfg *JcardConfig

			BeforeEach(func() {
				dir := GinkgoT().TempDir()
				tomlContent := `
mixtape = "base"

[[agents]]
harness = "claude-code"
prompt = "task one"

[[agents]]
harness = "claude-code"
prompt = "task two"
`
				cfgPath := filepath.Join(dir, "jcard.toml")
				Expect(os.WriteFile(cfgPath, []byte(tomlContent), 0644)).To(Succeed())

				var err error
				cfg, err = Load(cfgPath)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should generate unique names", func() {
				Expect(cfg.Agents[0].Name).To(Equal("claude-code-0"))
				Expect(cfg.Agents[1].Name).To(Equal("claude-code-1"))
			})
		})
	})

	Describe("Replicas", func() {
		It("should default replicas to 1", func() {
			dir := GinkgoT().TempDir()
			tomlContent := `
mixtape = "base"

[[agents]]
harness = "claude-code"
`
			cfgPath := filepath.Join(dir, "jcard.toml")
			Expect(os.WriteFile(cfgPath, []byte(tomlContent), 0644)).To(Succeed())

			cfg, err := Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Agents).To(HaveLen(1))
			Expect(cfg.Agents[0].Replicas).To(Equal(1))
			Expect(cfg.Agents[0].Name).To(Equal("claude-code"))
		})

		It("should expand replicas=1 without suffix", func() {
			dir := GinkgoT().TempDir()
			tomlContent := `
mixtape = "base"

[[agents]]
name = "reviewer"
harness = "claude-code"
replicas = 1
`
			cfgPath := filepath.Join(dir, "jcard.toml")
			Expect(os.WriteFile(cfgPath, []byte(tomlContent), 0644)).To(Succeed())

			cfg, err := Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Agents).To(HaveLen(1))
			Expect(cfg.Agents[0].Name).To(Equal("reviewer"))
		})

		It("should expand named replicas with index suffix", func() {
			dir := GinkgoT().TempDir()
			tomlContent := `
mixtape = "base"

[[agents]]
name = "reviewer"
harness = "claude-code"
prompt = "review code"
replicas = 3
`
			cfgPath := filepath.Join(dir, "jcard.toml")
			Expect(os.WriteFile(cfgPath, []byte(tomlContent), 0644)).To(Succeed())

			cfg, err := Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Agents).To(HaveLen(3))
			Expect(cfg.Agents[0].Name).To(Equal("reviewer-0"))
			Expect(cfg.Agents[1].Name).To(Equal("reviewer-1"))
			Expect(cfg.Agents[2].Name).To(Equal("reviewer-2"))
			// Each replica should have the same harness and prompt.
			for _, a := range cfg.Agents {
				Expect(a.Harness).To(Equal("claude-code"))
				Expect(a.Prompt).To(Equal("review code"))
				Expect(a.Replicas).To(Equal(1))
			}
		})

		It("should expand unnamed replicas and auto-generate names", func() {
			dir := GinkgoT().TempDir()
			tomlContent := `
mixtape = "base"

[[agents]]
harness = "claude-code"
replicas = 3
`
			cfgPath := filepath.Join(dir, "jcard.toml")
			Expect(os.WriteFile(cfgPath, []byte(tomlContent), 0644)).To(Succeed())

			cfg, err := Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Agents).To(HaveLen(3))
			Expect(cfg.Agents[0].Name).To(Equal("claude-code-0"))
			Expect(cfg.Agents[1].Name).To(Equal("claude-code-1"))
			Expect(cfg.Agents[2].Name).To(Equal("claude-code-2"))
		})

		It("should handle mixed replicas and single agents", func() {
			dir := GinkgoT().TempDir()
			tomlContent := `
mixtape = "base"

[[agents]]
name = "lead"
harness = "claude-code"

[[agents]]
name = "worker"
harness = "opencode"
replicas = 3
`
			cfgPath := filepath.Join(dir, "jcard.toml")
			Expect(os.WriteFile(cfgPath, []byte(tomlContent), 0644)).To(Succeed())

			cfg, err := Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Agents).To(HaveLen(4))
			Expect(cfg.Agents[0].Name).To(Equal("lead"))
			Expect(cfg.Agents[1].Name).To(Equal("worker-0"))
			Expect(cfg.Agents[2].Name).To(Equal("worker-1"))
			Expect(cfg.Agents[3].Name).To(Equal("worker-2"))
		})

		It("should deep-copy env maps across replicas", func() {
			dir := GinkgoT().TempDir()
			tomlContent := `
mixtape = "base"

[[agents]]
name = "worker"
harness = "claude-code"
replicas = 2

[agents.env]
KEY = "value"
`
			cfgPath := filepath.Join(dir, "jcard.toml")
			Expect(os.WriteFile(cfgPath, []byte(tomlContent), 0644)).To(Succeed())

			cfg, err := Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Agents).To(HaveLen(2))
			// Verify both have the env var.
			Expect(cfg.Agents[0].Env).To(HaveKeyWithValue("KEY", "value"))
			Expect(cfg.Agents[1].Env).To(HaveKeyWithValue("KEY", "value"))
		})

		It("should support large replica counts", func() {
			dir := GinkgoT().TempDir()
			tomlContent := `
mixtape = "base"

[[agents]]
name = "swarm"
harness = "claude-code"
replicas = 500
`
			cfgPath := filepath.Join(dir, "jcard.toml")
			Expect(os.WriteFile(cfgPath, []byte(tomlContent), 0644)).To(Succeed())

			cfg, err := Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Agents).To(HaveLen(500))
			Expect(cfg.Agents[0].Name).To(Equal("swarm-0"))
			Expect(cfg.Agents[499].Name).To(Equal("swarm-499"))
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

[[agents]]
harness = "invalid-harness"
`),
			Entry("invalid restart policy", `
mixtape = "base"

[[agents]]
harness = "claude-code"
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
			Entry("duplicate agent names", `
mixtape = "base"

[[agents]]
name = "same"
harness = "claude-code"

[[agents]]
name = "same"
harness = "opencode"
`),
			Entry("invalid agent type", `
mixtape = "base"

[[agents]]
harness = "claude-code"
type = "docker"
`),
			Entry("extra_packages with empty entry", `
mixtape = "base"

[[agents]]
harness = "claude-code"
extra_packages = ["ripgrep", "", "fd"]
`),
			Entry("extra_packages on native agent", `
mixtape = "base"

[[agents]]
harness = "claude-code"
type = "native"
extra_packages = ["ripgrep"]
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

[[agents]]
harness = "claude-code"
`
			cfgPath := filepath.Join(dir, "jcard.toml")
			Expect(os.WriteFile(cfgPath, []byte(tomlContent), 0644)).To(Succeed())

			cfg, err := Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Agents[0].Workdir).To(Equal("/code"))
		})
	})

	Describe("Agent type", func() {
		It("should parse type=sandboxed explicitly", func() {
			dir := GinkgoT().TempDir()
			tomlContent := `
mixtape = "base"

[[agents]]
harness = "claude-code"
type = "sandboxed"
`
			cfgPath := filepath.Join(dir, "jcard.toml")
			Expect(os.WriteFile(cfgPath, []byte(tomlContent), 0644)).To(Succeed())

			cfg, err := Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Agents[0].Type).To(Equal(AgentTypeSandboxed))
		})

		It("should parse type=native", func() {
			dir := GinkgoT().TempDir()
			tomlContent := `
mixtape = "base"

[[agents]]
harness = "claude-code"
type = "native"
`
			cfgPath := filepath.Join(dir, "jcard.toml")
			Expect(os.WriteFile(cfgPath, []byte(tomlContent), 0644)).To(Succeed())

			cfg, err := Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Agents[0].Type).To(Equal(AgentTypeNative))
		})
	})

	Describe("Extra packages", func() {
		It("should parse extra_packages for sandboxed agents", func() {
			dir := GinkgoT().TempDir()
			tomlContent := `
mixtape = "base"

[[agents]]
harness = "claude-code"
type = "sandboxed"
extra_packages = ["ripgrep", "fd", "python311"]
`
			cfgPath := filepath.Join(dir, "jcard.toml")
			Expect(os.WriteFile(cfgPath, []byte(tomlContent), 0644)).To(Succeed())

			cfg, err := Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Agents[0].ExtraPackages).To(Equal([]string{"ripgrep", "fd", "python311"}))
		})

		It("should accept empty extra_packages list", func() {
			dir := GinkgoT().TempDir()
			tomlContent := `
mixtape = "base"

[[agents]]
harness = "claude-code"
extra_packages = []
`
			cfgPath := filepath.Join(dir, "jcard.toml")
			Expect(os.WriteFile(cfgPath, []byte(tomlContent), 0644)).To(Succeed())

			cfg, err := Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Agents[0].ExtraPackages).To(BeEmpty())
		})

		It("should accept sandboxed agent without extra_packages", func() {
			dir := GinkgoT().TempDir()
			tomlContent := `
mixtape = "base"

[[agents]]
harness = "claude-code"
type = "sandboxed"
`
			cfgPath := filepath.Join(dir, "jcard.toml")
			Expect(os.WriteFile(cfgPath, []byte(tomlContent), 0644)).To(Succeed())

			cfg, err := Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Agents[0].ExtraPackages).To(BeNil())
		})

		It("should round-trip extra_packages through Marshal", func() {
			cfg := &JcardConfig{
				Mixtape: "base",
				Agents: []AgentConfig{
					{
						Harness:       "claude-code",
						ExtraPackages: []string{"ripgrep", "fd"},
					},
				},
			}
			data, err := Marshal(cfg)
			Expect(err).NotTo(HaveOccurred())

			dir := GinkgoT().TempDir()
			cfgPath := filepath.Join(dir, "jcard.toml")
			Expect(os.WriteFile(cfgPath, data, 0644)).To(Succeed())

			loaded, err := Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(loaded.Agents[0].ExtraPackages).To(Equal([]string{"ripgrep", "fd"}))
		})
	})
})
