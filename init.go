package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)


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

	// Snapshot agent needs Bash to test the script it writes
	allowedTools := "Read,Write,Edit,Glob,Grep"
	if role == "snapshot" {
		allowedTools = "Read,Write,Edit,Glob,Grep,Bash"
	}

	runAgent := func() (int, error) {
		switch tool {
		case "claude":
			return runPipeQuiet(prompt, logFile, "claude", "-p", "--allowedTools", allowedTools)
		case "amp":
			return runPipeQuiet(prompt, logFile, "amp", "--dangerously-allow-all")
		default:
			return 1, fmt.Errorf("unknown tool: %s", tool)
		}
	}

	if interactive {
		switch tool {
		case "claude":
			return run("claude", prompt, "--allowedTools", allowedTools)
		case "amp":
			return run("amp", "--dangerously-allow-all")
		default:
			return fmt.Errorf("unknown tool: %s", tool)
		}
	}

	for {
		stop := spinner(role)
		_, err := runAgent()
		stop()

		// Check for rate limit in output
		if data, readErr := os.ReadFile(logFile); readErr == nil {
			if strings.Contains(string(data), "You've hit your limit") {
				sleepUntilRateLimitLifts()
				continue // retry this role
			}
		}

		return err
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
		// \033[K clears from cursor to end of line, wiping any trailing chars
		// from the longer running-spinner line ("Generating ... NmSSs" vs final "(NmSSs)").
		fmt.Fprintf(os.Stderr, "\r\033[K  ✓  Generated %s agent (%dm%02ds)\n", role, mins, secs)
	}
}

func cmdInit(args []string) {
	tool := "claude"
	interactive := false
	targetAgent := ""

	for i := 0; i < len(args); i++ {
		arg := args[i]
		// Support --flag=value syntax
		key, val := arg, ""
		if eq := strings.IndexByte(arg, '='); eq >= 0 {
			key, val = arg[:eq], arg[eq+1:]
		}
		switch key {
		case "--tool":
			if val != "" {
				tool = val
			} else {
				i++
				if i < len(args) {
					tool = args[i]
				}
			}
		case "--interactive":
			interactive = true
		case "--agent":
			if val != "" {
				targetAgent = val
			} else {
				i++
				if i < len(args) {
					targetAgent = args[i]
				}
			}
		default:
			die("Unknown option: %s", args[i])
		}
	}

	// Validate --agent
	if targetAgent != "" {
		// Accept "goal" as a transitional alias so existing muscle memory still works.
		if targetAgent == "goal" {
			targetAgent = "champion"
		}
		switch targetAgent {
		case "snapshot", "champion", "brand", "builder", "verifier":
		default:
			die("Unknown agent role: %s (expected: snapshot, champion, brand, builder, verifier)", targetAgent)
		}
	}

	// Targeted re-init
	if targetAgent != "" {
		if _, err := os.Stat(latheDir); os.IsNotExist(err) {
			die(".lathe/ not found — run 'lathe init' first (without --agent)")
		}
		os.MkdirAll(latheAgents, 0755)
		fmt.Printf("  Re-initializing %s agent only.\n\n", targetAgent)

		if err := generateAgentRole(targetAgent, tool, interactive); err != nil {
			die("%s agent generation failed: %v", targetAgent, err)
		}

		if targetAgent == "snapshot" {
			// Ensure snapshot.sh is executable
			os.Chmod(filepath.Join(latheDir, "snapshot.sh"), 0755)
			fmt.Printf("  Updated: %s/snapshot.sh\n", latheDir)
		} else {
			fmt.Printf("  Updated: %s/%s.md\n", latheAgents, targetAgent)
		}
		fmt.Println()
		fmt.Println("  Note: downstream agents may need re-init too.")
		fmt.Println("  (snapshot → champion → brand → builder → verifier)")
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
		fmt.Println("  LATHE — initializing project")
	}
	fmt.Println()

	os.MkdirAll(latheAgents, 0755)
	os.MkdirAll(filepath.Join(latheDir, "skills"), 0755)
	os.MkdirAll(filepath.Join(latheDir, "refs"), 0755)

	// Generate snapshot + four agent roles in sequence.
	// Ordering matters: brand reads champion.md's stakeholder map; builder and verifier read brand.md.
	roles := []string{"snapshot", "champion", "brand", "builder", "verifier"}
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

	// Ensure snapshot.sh is executable
	os.Chmod(filepath.Join(latheDir, "snapshot.sh"), 0755)

	// Validate
	if _, err := os.Stat(filepath.Join(latheDir, "snapshot.sh")); os.IsNotExist(err) {
		fmt.Println()
		fmt.Printf("  ERROR: Agent generation produced unusable output.\n")
		fmt.Printf("  Missing: %s/snapshot.sh\n", latheDir)
		if !reinit {
			os.RemoveAll(latheDir)
		}
		os.Exit(1)
	}
	if _, err := os.Stat(filepath.Join(latheAgents, "champion.md")); os.IsNotExist(err) {
		fmt.Println()
		fmt.Printf("  ERROR: Agent generation produced unusable output.\n")
		fmt.Printf("  Missing: %s/champion.md\n", latheAgents)
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
		fmt.Println("  Updated: agents/{champion,brand,builder,verifier}.md, snapshot.sh, skills")
	} else {
		fmt.Printf("  Created: %s/\n", latheDir)
	}
	fmt.Printf("  Champion: %s/champion.md\n", latheAgents)
	fmt.Printf("  Brand:    %s/brand.md\n", latheAgents)
	fmt.Printf("  Builder:  %s/builder.md\n", latheAgents)
	fmt.Printf("  Verify:   %s/verifier.md\n", latheAgents)
	fmt.Printf("  Skills:   %s/\n", latheSkills)
	fmt.Printf("  Snap:     %s/snapshot.sh\n", latheDir)

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
