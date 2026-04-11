package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func detectType() string {
	if _, err := os.Stat("go.mod"); err == nil {
		return "go"
	}
	if _, err := os.Stat("Cargo.toml"); err == nil {
		return "rust"
	}
	if _, err := os.Stat("package.json"); err == nil {
		return "node"
	}
	if _, err := os.Stat("requirements.txt"); err == nil {
		return "python"
	}
	if _, err := os.Stat("pyproject.toml"); err == nil {
		return "python"
	}
	entries, _ := filepath.Glob("*.yaml")
	for _, e := range entries {
		data, _ := os.ReadFile(e)
		if strings.Contains(string(data), "apiVersion:") {
			return "k8s"
		}
	}
	entries, _ = filepath.Glob("*.yml")
	for _, e := range entries {
		data, _ := os.ReadFile(e)
		if strings.Contains(string(data), "apiVersion:") {
			return "k8s"
		}
	}
	return "generic"
}

func generateAgentRole(role, tool string, interactive bool) error {
	tpl, err := templatesFS.ReadFile("templates/meta-" + role + ".md")
	if err != nil {
		return fmt.Errorf("no meta-prompt for role: %s", role)
	}

	prompt := string(tpl)

	// Splice values manifesto if placeholder present
	if data, err := templatesFS.ReadFile("templates/values-manifesto.md"); err == nil {
		prompt = strings.ReplaceAll(prompt, "{{VALUES_MANIFESTO}}", string(data))
	}

	// Splice interactive preamble
	if interactive {
		if data, err := templatesFS.ReadFile("templates/interactive-preamble.md"); err == nil {
			prompt = strings.ReplaceAll(prompt, "{{INTERACTIVE}}", string(data))
		}
	} else {
		prompt = strings.ReplaceAll(prompt, "{{INTERACTIVE}}", "")
	}

	logFile := filepath.Join(latheDir, "init-"+role+".log")

	switch tool {
	case "claude":
		if interactive {
			return run("claude", prompt, "--allowedTools", "Read,Write,Edit,Glob,Grep")
		}
		stop := spinner(role)
		_, err := runPipeQuiet(prompt, logFile, "claude", "-p", "--allowedTools", "Read,Write,Edit,Glob,Grep")
		stop()
		return err
	case "amp":
		if interactive {
			return run("amp", "--dangerously-allow-all")
		}
		stop := spinner(role)
		_, err := runPipeQuiet(prompt, logFile, "amp", "--dangerously-allow-all")
		stop()
		return err
	default:
		return fmt.Errorf("unknown tool: %s", tool)
	}
}

// spinner shows an animated spinner with elapsed time while an agent runs.
// Returns a stop function that clears the spinner line.
func spinner(role string) func() {
	done := make(chan struct{})
	start := time.Now()
	go func() {
		frames := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
		i := 0
		for {
			select {
			case <-done:
				return
			default:
				elapsed := time.Since(start).Truncate(time.Second)
				mins := int(elapsed.Minutes())
				secs := int(elapsed.Seconds()) % 60
				fmt.Fprintf(os.Stderr, "\r  %c  Generating %s agent ... %dm%02ds", frames[i%len(frames)], role, mins, secs)
				i++
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()
	return func() {
		close(done)
		elapsed := time.Since(start).Truncate(time.Second)
		mins := int(elapsed.Minutes())
		secs := int(elapsed.Seconds()) % 60
		fmt.Fprintf(os.Stderr, "\r  ✓  Generated %s agent (%dm%02ds)\n", role, mins, secs)
	}
}

func cmdInit(args []string) {
	projectType := ""
	tool := "claude"
	interactive := false
	targetAgent := ""

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--type":
			i++
			if i < len(args) {
				projectType = args[i]
			}
		case "--tool":
			i++
			if i < len(args) {
				tool = args[i]
			}
		case "--interactive":
			interactive = true
		case "--agent":
			i++
			if i < len(args) {
				targetAgent = args[i]
			}
		default:
			die("Unknown option: %s", args[i])
		}
	}

	// Validate --agent
	if targetAgent != "" {
		switch targetAgent {
		case "goal", "builder", "verifier":
		default:
			die("Unknown agent role: %s (expected: goal, builder, verifier)", targetAgent)
		}
	}

	if projectType == "" {
		projectType = detectType()
		fmt.Printf("  Detected project type: %s\n", projectType)
	}

	// Check if embedded templates exist for this project type
	snapshotPath := "templates/" + projectType + "/snapshot.sh"
	if _, err := templatesFS.ReadFile(snapshotPath); err != nil {
		fmt.Printf("  No template for '%s', falling back to generic\n", projectType)
		projectType = "generic"
		snapshotPath = "templates/generic/snapshot.sh"
	}

	// Targeted re-init
	if targetAgent != "" {
		if _, err := os.Stat(latheDir); os.IsNotExist(err) {
			die(".lathe/ not found — run 'lathe init' first (without --agent)")
		}
		fmt.Printf("  Re-initializing %s agent only.\n\n", targetAgent)

		if err := generateAgentRole(targetAgent, tool, interactive); err != nil {
			die("%s agent generation failed: %v", targetAgent, err)
		}

		fmt.Printf("  Updated: %s/%s.md\n", latheDir, targetAgent)
		fmt.Println()
		fmt.Println("  Note: downstream agents may need re-init too.")
		fmt.Println("  (goal → builder → verifier)")
		return
	}

	// Full init
	reinit := false
	if _, err := os.Stat(latheDir); err == nil {
		reinit = true
		fmt.Println("  Re-initializing (preserving refs/).")

		entries, _ := os.ReadDir(latheDir)
		for _, e := range entries {
			if e.Name() == "refs" {
				continue
			}
			os.RemoveAll(filepath.Join(latheDir, e.Name()))
		}
	} else {
		fmt.Println()
		fmt.Println("  ╔═══════════════════════════════════════════╗")
		fmt.Println("  ║  LATHE — initializing project              ║")
		fmt.Println("  ╚═══════════════════════════════════════════╝")
	}
	fmt.Println()

	os.MkdirAll(filepath.Join(latheDir, "skills"), 0755)
	os.MkdirAll(filepath.Join(latheDir, "refs"), 0755)

	// Copy snapshot from embedded FS
	snapshotDst := filepath.Join(latheDir, "snapshot.sh")
	if data, err := templatesFS.ReadFile(snapshotPath); err == nil {
		os.WriteFile(snapshotDst, data, 0755)
	}

	// Generate three agent roles in sequence
	roles := []string{"goal", "builder", "verifier"}
	for _, role := range roles {
		if err := generateAgentRole(role, tool, interactive); err != nil {
			fmt.Println()
			fmt.Printf("  ERROR: %s agent generation failed: %v\n", role, err)
			if data, err := os.ReadFile(filepath.Join(latheDir, "init-"+role+".log")); err == nil {
				lines := strings.Split(string(data), "\n")
				start := len(lines) - 20
				if start < 0 {
					start = 0
				}
				fmt.Println()
				fmt.Printf("  --- last 20 lines of init-%s.log ---\n", role)
				for _, line := range lines[start:] {
					fmt.Printf("  %s\n", line)
				}
				fmt.Println("  --- end ---")
			}
			fmt.Println()
			fmt.Printf("  You can retry with: lathe init --tool %s\n", tool)
			fmt.Printf("  Or retry just this role: lathe init --agent %s\n", role)
			if !reinit {
				os.RemoveAll(latheDir)
			}
			os.Exit(1)
		}
	}

	// Validate
	if _, err := os.Stat(filepath.Join(latheDir, "goal.md")); os.IsNotExist(err) {
		fmt.Println()
		fmt.Println("  ERROR: Agent generation produced unusable output.")
		fmt.Println("  The AI ran but didn't produce a valid goal.md.")
		if !reinit {
			os.RemoveAll(latheDir)
		}
		os.Exit(1)
	}

	fmt.Println("  Agents generated via " + tool + ".")

	// Ensure .lathe/session/ is gitignored
	ensureGitignore()

	// Install global skill
	installSkill()

	if reinit {
		fmt.Println("  Updated: goal.md, builder.md, verifier.md, snapshot.sh, skills")
	} else {
		fmt.Printf("  Created: %s/\n", latheDir)
	}
	fmt.Printf("  Goal:    %s/goal.md\n", latheDir)
	fmt.Printf("  Builder: %s/builder.md\n", latheDir)
	fmt.Printf("  Verify:  %s/verifier.md\n", latheDir)
	fmt.Printf("  Skills:  %s/skills/\n", latheDir)
	fmt.Printf("  Snap:    %s/snapshot.sh\n", latheDir)

	if _, err := os.Stat(filepath.Join(latheDir, "alignment-summary.md")); err == nil {
		fmt.Println()
		fmt.Printf("  Review the alignment summary before starting:\n")
		fmt.Printf("  cat %s/alignment-summary.md\n", latheDir)
		fmt.Println()
		fmt.Println("  If something looks off, re-run with: lathe init --interactive")
		fmt.Println("  Or target a specific role: lathe init --agent goal")
	}
	fmt.Println()
	fmt.Println("  Run 'lathe start' to begin.")
}

func ensureGitignore() {
	gitignore := ".gitignore"
	data, _ := os.ReadFile(gitignore)
	if strings.Contains(string(data), ".lathe/session/") {
		return
	}
	f, err := os.OpenFile(gitignore, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, "")
	fmt.Fprintln(f, "# Lathe session state (ephemeral, engine-managed)")
	fmt.Fprintln(f, ".lathe/session/")
}

func installSkill() {
	skillDir := filepath.Join(os.Getenv("HOME"), ".claude", "skills", "lathe")
	dst := filepath.Join(skillDir, "SKILL.md")

	srcData, err := templatesFS.ReadFile("templates/skill/SKILL.md")
	if err != nil {
		return
	}

	dstData, _ := os.ReadFile(dst)
	if string(srcData) == string(dstData) {
		return
	}

	os.MkdirAll(skillDir, 0755)
	os.WriteFile(dst, srcData, 0644)
	fmt.Println("  Skill:   ~/.claude/skills/lathe/ (updated)")
}
