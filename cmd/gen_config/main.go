// Package main provides the gen_config utility for encoding configuration strings.
package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
)

// encodeConfig applies the reverse transformation of "tr 012. .201"
// Forward (client): 0->. 1->2 2->0 .->1
// Reverse (generator): .->0 2->1 0->2 1->.
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

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: gen_config \"<config_string>\"")
		fmt.Println("Example: gen_config \"-port=10553 127.0.0.1\"")
		os.Exit(1)
	}

	input := strings.Join(os.Args[1:], " ")
	encoded := encodeConfig(input)
	fmt.Print(encoded)
}
