package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func testCert(t *testing.T) tls.Certificate {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "smgw.local"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// The ERC sends a 48 byte NTP v3 client packet inside TLS 1.2 and expects a server reply.
func TestNtpOverTLS(t *testing.T) {
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{testCert(t)}, MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err == nil {
			handleNtpOverTLS(c)
		}
	}()
	c, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{InsecureSkipVerify: true, MaxVersion: tls.VersionTLS12,
		CipherSuites: []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	req := make([]byte, 48)
	req[0] = 0x1b
	binary.BigEndian.PutUint64(req[40:48], 0x0000000a265da248)
	if _, err := c.Write(req); err != nil {
		t.Fatal(err)
	}
	resp := make([]byte, 48)
	if n, err := io.ReadFull(c, resp); err != nil {
		t.Fatalf("read %d bytes: %v (state %+v)", n, err, c.ConnectionState().HandshakeComplete)
	}
	if resp[0]&7 != 4 || (resp[0]>>3)&7 != 3 {
		t.Fatalf("mode/version %08b", resp[0])
	}
	if binary.BigEndian.Uint64(resp[24:32]) != 0x0000000a265da248 {
		t.Fatal("originate timestamp not echoed")
	}
	sec := int64(binary.BigEndian.Uint32(resp[40:44])) - ntpUnixDelta
	if d := time.Since(time.Unix(sec, 0)); d < -2*time.Second || d > 2*time.Second {
		t.Fatalf("transmit time off by %v", d)
	}
}

// A legacy unicast query (source port != 5353) for smgw.local is answered directly.
func TestSmgwMdns(t *testing.T) {
	ip := ifaceIPv4ForTest(t)
	go serveSmgwMdns(net.IPv4(192, 0, 2, 1).To4())
	time.Sleep(300 * time.Millisecond)
	q := new(dns.Msg)
	q.SetQuestion(smgwName, dns.TypeA)
	q.RecursionDesired = false
	out, _ := q.Pack()
	c, err := net.ListenPacket("udp4", ip.String()+":0")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	buf := make([]byte, 1500)
	// Port 5353 is shared with Avahi, so a unicast query may land there instead: try a few times.
	for i := 0; i < 10; i++ {
		_, _ = c.WriteTo(out, &net.UDPAddr{IP: ip, Port: 5353})
		_ = c.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
		for {
			n, _, err := c.ReadFrom(buf)
			if err != nil {
				break
			}
			var r dns.Msg
			if r.Unpack(buf[:n]) != nil {
				continue
			}
			for _, rr := range r.Answer {
				if a, ok := rr.(*dns.A); ok && a.A.Equal(net.IPv4(192, 0, 2, 1)) && r.Id == q.Id {
					return
				}
			}
		}
	}
	t.Fatal("no answer for smgw.local")
}

func ifaceIPv4ForTest(t *testing.T) net.IP {
	ifaces, _ := net.Interfaces()
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 || ifi.Flags&net.FlagMulticast == 0 {
			continue
		}
		if ip := ifaceIPv4(ifi.Index); ip != nil {
			return ip
		}
	}
	t.Skip("no IPv4 interface")
	return nil
}

// The built frontend is served on the websocket port, so one process provides both.
func TestFrontendServed(t *testing.T) {
	if _, err := os.Stat(frontendDir + "/index.html"); err != nil {
		t.Skip("frontend not built")
	}
	setupRoutes(nil)
	srv := httptest.NewServer(http.DefaultServeMux)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "<div id=\"app\"") {
		t.Fatalf("status %d, body %.80q", resp.StatusCode, body)
	}
}
