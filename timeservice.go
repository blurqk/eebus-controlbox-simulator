package main

// Time service for the Weidmueller EEBUS Relais Converter (ERC).
//
// At boot the ERC looks for its smart meter gateway by an mDNS query for
// "smgw.local", opens a TLS 1.2 connection to TCP port 4460 and sends a plain
// 48 byte NTP client packet inside it. It does not verify the server
// certificate. Without an answer it logs "PPCCLS_TIMESYNC_CONNECT: could not
// connect to peer" and its clock stays at 1970.
//
// Enabled by default, SMGW_TIME=0 switches it off. SMGW_IP overrides the IPv4
// address announced for smgw.local (default: the address of the interface the
// query arrived on). Only one device on the network should answer smgw.local:
// run either this simulator or the Tasmota one (EEBusSmgw), not both.

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/miekg/dns"
	"golang.org/x/net/ipv4"
	"golang.org/x/sys/unix"
)

const (
	smgwName     = "smgw.local."
	smgwTimePort = 4460
	ntpUnixDelta = 2208988800
)

var mdnsGroup = &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}

func startTimeService(cert tls.Certificate) {
	var fixedIP net.IP
	if s := os.Getenv("SMGW_IP"); s != "" {
		if fixedIP = net.ParseIP(s).To4(); fixedIP == nil {
			log.Fatalf("SMGW_IP: invalid IPv4 address %q", s)
		}
	}
	go serveSmgwMdns(fixedIP)
	go serveNtpOverTLS(cert)
}

// serveSmgwMdns answers mDNS A queries for smgw.local.
func serveSmgwMdns(fixedIP net.IP) {
	// Share port 5353 with a running Avahi daemon.
	lc := net.ListenConfig{Control: func(_, _ string, rc syscall.RawConn) error {
		var serr error
		_ = rc.Control(func(fd uintptr) {
			if serr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); serr == nil {
				serr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
			}
		})
		return serr
	}}
	c, err := lc.ListenPacket(context.Background(), "udp4", "0.0.0.0:5353")
	if err != nil {
		log.Printf("smgw.local: mDNS listen failed: %v", err)
		return
	}
	p := ipv4.NewPacketConn(c)
	_ = p.SetControlMessage(ipv4.FlagInterface, true)
	_ = p.SetMulticastTTL(255)
	ifaces, _ := net.Interfaces()
	joined := 0
	for i := range ifaces {
		ifi := &ifaces[i]
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagMulticast == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		if p.JoinGroup(ifi, mdnsGroup) == nil {
			joined++
		}
	}
	log.Printf("smgw.local: answering mDNS on %d interfaces", joined)

	buf := make([]byte, 9000)
	for {
		n, cm, src, err := p.ReadFrom(buf)
		if err != nil {
			log.Printf("smgw.local: mDNS read failed: %v", err)
			return
		}
		var q dns.Msg
		if q.Unpack(buf[:n]) != nil || q.Response {
			continue
		}
		for _, qs := range q.Question {
			if !strings.EqualFold(qs.Name, smgwName) || (qs.Qtype != dns.TypeA && qs.Qtype != dns.TypeANY) {
				continue
			}
			ip := fixedIP
			if ip == nil && cm != nil {
				ip = ifaceIPv4(cm.IfIndex)
			}
			if ip == nil {
				continue
			}
			resp := new(dns.Msg)
			resp.Response = true
			resp.Authoritative = true
			resp.Answer = []dns.RR{&dns.A{
				Hdr: dns.RR_Header{Name: smgwName, Rrtype: dns.TypeA, Class: dns.ClassINET | 1<<15, Ttl: 120},
				A:   ip,
			}}
			out, err := resp.Pack()
			if err != nil {
				continue
			}
			// Legacy unicast query (source port != 5353) or unicast-response bit set: answer
			// the sender directly, otherwise to the multicast group on the arrival interface.
			usrc, _ := src.(*net.UDPAddr)
			var wcm *ipv4.ControlMessage
			dst := net.Addr(mdnsGroup)
			if (usrc != nil && usrc.Port != 5353) || qs.Qclass&(1<<15) != 0 {
				dst = src
				if usrc != nil && usrc.Port != 5353 {
					resp.Id = q.Id
					resp.Question = q.Question
					out, _ = resp.Pack()
				}
			} else if cm != nil {
				wcm = &ipv4.ControlMessage{IfIndex: cm.IfIndex}
			}
			if _, err := p.WriteTo(out, wcm, dst); err == nil {
				log.Printf("smgw.local: answered %s with %s", src, ip)
			}
		}
	}
}

func ifaceIPv4(index int) net.IP {
	ifi, err := net.InterfaceByIndex(index)
	if err != nil {
		return nil
	}
	addrs, _ := ifi.Addrs()
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil {
			return n.IP.To4()
		}
	}
	return nil
}

// serveNtpOverTLS answers the ERC's NTP request inside TLS 1.2 on port 4460.
func serveNtpOverTLS(cert tls.Certificate) {
	ln, err := tls.Listen("tcp", ":4460", &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	})
	if err != nil {
		log.Printf("smgw.local: time service listen failed: %v", err)
		return
	}
	log.Printf("smgw.local: time service listening on port %d", smgwTimePort)
	for {
		c, err := ln.Accept()
		if err != nil {
			log.Printf("smgw.local: accept failed: %v", err)
			continue
		}
		go handleNtpOverTLS(c)
	}
}

func handleNtpOverTLS(c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	req := make([]byte, 48)
	if _, err := io.ReadFull(c, req); err != nil {
		log.Printf("smgw.local: time request from %s failed: %v", c.RemoteAddr(), err)
		return
	}
	rx := time.Now()
	version := (req[0] >> 3) & 7
	resp := make([]byte, 48)
	resp[0] = version<<3 | 4                      // LI 0, version as requested, mode 4 (server)
	resp[1] = 2                                   // stratum: synchronised via the host clock
	resp[2] = req[2]                              // poll interval as requested
	resp[3] = 0xEC                                // precision 2^-20 s
	binary.BigEndian.PutUint32(resp[8:12], 1<<12) // root dispersion ~ 1/16 s
	tx := time.Now()
	putNtpTime(resp[16:24], tx)   // reference time
	copy(resp[24:32], req[40:48]) // originate time = client's transmit time
	putNtpTime(resp[32:40], rx)   // receive time
	putNtpTime(resp[40:48], tx)   // transmit time
	if _, err := c.Write(resp); err != nil {
		log.Printf("smgw.local: time reply to %s failed: %v", c.RemoteAddr(), err)
		return
	}
	log.Printf("smgw.local: time sent to %s (NTP v%d, %s)", c.RemoteAddr(), version, tx.UTC().Format(time.RFC3339))
	// Send close_notify and let the client close first: closing at once can reset the
	// connection before the client has read the reply.
	if tc, ok := c.(*tls.Conn); ok {
		_ = tc.CloseWrite()
	}
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _ = io.Copy(io.Discard, c)
}

func putNtpTime(b []byte, t time.Time) {
	binary.BigEndian.PutUint32(b[0:4], uint32(t.Unix()+ntpUnixDelta))
	binary.BigEndian.PutUint32(b[4:8], uint32((uint64(t.Nanosecond())<<32)/1e9))
}
