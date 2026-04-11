package main

import (
	"os"
	"path/filepath"
)

// collectSnapshot runs .lathe/snapshot.sh and writes output to session/snapshot.txt.
func collectSnapshot() error {
	log("Collecting project snapshot ...")
	out := filepath.Join(latheSession, "snapshot.txt")

	script := filepath.Join(latheDir, "snapshot.sh")
	if _, err := os.Stat(script); os.IsNotExist(err) {
		return os.WriteFile(out, []byte("(no snapshot script found)\n"), 0644)
	}

	output, err := runCaptureAll(script)
	if err != nil {
		// Still write what we got — partial snapshot is better than none
		output += "\n(snapshot.sh exited with error: " + err.Error() + ")\n"
	}

	log("Snapshot written: %s", out)
	return os.WriteFile(out, []byte(output), 0644)
}
