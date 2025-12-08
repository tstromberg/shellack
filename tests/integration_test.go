package tests

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIntegration(t *testing.T) {
	// 1. Build the server
	serverBin := filepath.Join(t.TempDir(), "shellack-server")

	cmd := exec.Command("go", "build", "-o", serverBin, "github.com/tstromberg/shellack/cmd/server")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build server: %v\n%s", err, out)
	}
	defer func() {
		_ = os.Remove(serverBin) //nolint:errcheck // Ignore error on cleanup
	}()

	tests := []struct {
		name           string
		command        string
		expectedOutput []string
	}{
		{
			name:           "uname -a",
			command:        "uname -a",
			expectedOutput: []string{"Darwin", "Linux"},
		},
		{
			name:           "python3 hello",
			command:        "python3 -c \"print('Hello from Python!')\"",
			expectedOutput: []string{"Hello from Python!"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runTest(t, serverBin, tc.command, tc.expectedOutput)
		})
	}
}

func runTest(t *testing.T, serverBin, command string, expectedOutput []string) {
	t.Helper()

	// 2. Start the server
	// Use a random high port? Let's try 10553 + offset? Or just 0 (random).
	// Since we kill it after each test, reuse 10553 is fine IF we ensure it's dead.
	// But multiple runs might race on port release.
	// Let's use a dynamic port or retry?
	// For simplicity, assume OS releases port quickly or use different ports.
	// Let's use 10553 for first, 10554 for second?
	// Better: Let server pick random port? server code defaults to 53 or arg.
	// Let's just pick a random-ish port.
	// Or stick to 10553 and hope for the best.
	// Actually, we can pass "0" to server to pick random port, but client needs to know it.
	// Server prints "Starting DNS server on port ...". We could parse it.
	// But `server/main.go` takes port as arg and listens on it.
	// Let's increment port.
	// static port counter?

	// We can't easily share state between t.Run.
	// Let's just use a unique port per test case based on something.
	// Hash of name? Or just hardcode.

	port := "10553"
	if strings.Contains(command, "python") {
		port = "10554"
	}

	serverCmd := exec.Command(serverBin, port, command)

	// Capture server stderr (log output) to verify success
	serverOutPipe, err := serverCmd.StderrPipe()
	if err != nil {
		t.Fatalf("Failed to get stderr pipe: %v", err)
	}
	if err := serverCmd.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Kill server at end of test
	defer func() {
		if serverCmd.Process != nil {
			_ = serverCmd.Process.Kill() //nolint:errcheck // Ignore error on cleanup
		}
		_ = serverCmd.Wait() //nolint:errcheck // Ignore error on cleanup
	}()

	// Wait a bit for server to start
	time.Sleep(1 * time.Second)

	// 3. Setup configuration file for the client
	// Script looks for ${path}/${MAGIC_WORD}.aiff
	// MAGIC_WORD default is "shellack"
	// Paths: /Library/Preferences/Audio/Data /tmp
	// We'll use /tmp/shellack.aiff

	configPath := "/tmp/shellack.aiff"
	// The content must be encoded using the gen_config tool logic
	targetConfig := fmt.Sprintf("-port=%s 127.0.0.1", port)

	// Use the gen_config tool to encode
	genCmd := exec.Command("go", "run", "github.com/tstromberg/shellack/cmd/gen_config", targetConfig)
	encodedBytes, err := genCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run gen_config: %v\nOutput: %s", err, encodedBytes)
	}

	if err := os.WriteFile(configPath, encodedBytes, 0o644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	defer func() {
		_ = os.Remove(configPath) //nolint:errcheck // Ignore error on cleanup
	}()

	// 4. Run the shell script
	// We run it with a timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	scriptPath := "../shellack.sh"
	clientCmd := exec.CommandContext(ctx, "/bin/sh", scriptPath)
	clientCmd.Stdout = os.Stdout
	clientCmd.Stderr = os.Stderr

	if err := clientCmd.Start(); err != nil {
		t.Fatalf("Failed to start client script: %v", err)
	}

	// 5. Monitor server output for success
	done := make(chan bool)
	go func() {
		scanner := bufio.NewScanner(serverOutPipe)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Println("[SERVER LOG]", line) // Forward to test output
			if strings.Contains(line, "decode <nil>:") {
				// Success!
				// Verify content
				found := false
				for _, expected := range expectedOutput {
					if strings.Contains(line, expected) {
						found = true
						break
					}
				}
				if found {
					done <- true
					return
				}
			}
		}
	}()

	select {
	case <-done:
		t.Logf("Test passed: Server received and decoded payload matching expected output.")
	case <-ctx.Done():
		t.Fatal("Test failed: Timeout waiting for server to decode payload.")
	case <-time.After(20 * time.Second): // Safety net
		t.Fatal("Test failed: Timed out.")
	}
}
