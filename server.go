package main

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// DNSServer represents a DNS server instance
type DNSServer struct {
	config    *Config
	server    *dns.Server
	client    *dns.Client
	upstreams map[string]*dns.Client
}

// NewDNSServer creates a new DNS server with the given configuration
func NewDNSServer(config *Config) *DNSServer {
	dnsServer := &DNSServer{
		config:    config,
		upstreams: make(map[string]*dns.Client),
	}

	// Initialize upstream clients
	for name, upstream := range config.Upstreams {
		client := &dns.Client{
			Net:          upstream.Protocol,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
		}
		dnsServer.upstreams[name] = client
	}

	return dnsServer
}

// Start starts the DNS server
func (s *DNSServer) Start() error {
	// Create a new DNS server
	addr := fmt.Sprintf("%s:%d", s.config.Server.Listen, s.config.Server.Port)
	s.server = &dns.Server{
		Addr:    addr,
		Net:     "udp",
		Handler: dns.HandlerFunc(s.handleRequest),
	}

	log.Printf("Starting DNS server on %s\n", addr)
	return s.server.ListenAndServe()
}

// Stop stops the DNS server
func (s *DNSServer) Stop() error {
	if s.server != nil {
		return s.server.Shutdown()
	}
	return nil
}

// handleRequest processes incoming DNS requests
func (s *DNSServer) handleRequest(w dns.ResponseWriter, r *dns.Msg) {
	if len(r.Question) == 0 {
		s.sendServerFailure(w, r, fmt.Errorf("empty question section"))
		return
	}

	q := r.Question[0]

	// Log query if enabled
	if s.config.Server.LogQueries {
		log.Printf("Query: %s, Type: %s", q.Name, dns.TypeToString[q.Qtype])
	}

	// Try to respond from local records first
	if s.handleLocalRecord(w, r, q) {
		return
	}

	// Forward to upstream if no local record found
	s.handleUpstreamRequest(w, r)
}

// handleLocalRecord attempts to respond using a local DNS record
// Returns true if a local record was found and used
func (s *DNSServer) handleLocalRecord(w dns.ResponseWriter, r *dns.Msg, q dns.Question) bool {
	recordType := dns.TypeToString[q.Qtype]
	domain := getDomainFromQuestion(q)

	record := FindMatchingRecord(domain, recordType)
	if record == nil {
		return false
	}

	// Create response
	m := new(dns.Msg)
	m.SetReply(r)

	// Add appropriate record to answer
	s.addRecordToMsg(m, q, record, recordType)

	// Only send if we added an answer
	if len(m.Answer) > 0 {
		if s.config.Server.LogQueries {
			log.Printf("Response for %s from local records: %s", domain, recordType)
		}
		w.WriteMsg(m)
		return true
	}

	return false
}

// addRecordToMsg adds the appropriate DNS record to the message based on record type
func (s *DNSServer) addRecordToMsg(m *dns.Msg, q dns.Question, record *RecordEntry, recordType string) {
	header := dns.RR_Header{
		Name:  q.Name,
		Class: dns.ClassINET,
		Ttl:   uint32(record.TTL),
	}

	switch recordType {
	case "A":
		header.Rrtype = dns.TypeA
		m.Answer = append(m.Answer, &dns.A{
			Hdr: header,
			A:   net.ParseIP(record.Value),
		})
	case "AAAA":
		header.Rrtype = dns.TypeAAAA
		m.Answer = append(m.Answer, &dns.AAAA{
			Hdr:  header,
			AAAA: net.ParseIP(record.Value),
		})
	case "CNAME":
		header.Rrtype = dns.TypeCNAME
		m.Answer = append(m.Answer, &dns.CNAME{
			Hdr:    header,
			Target: dns.Fqdn(record.Value),
		})
	case "TXT":
		header.Rrtype = dns.TypeTXT
		m.Answer = append(m.Answer, &dns.TXT{
			Hdr: header,
			Txt: []string{record.Value},
		})
	case "MX":
		header.Rrtype = dns.TypeMX
		priority, target := s.parseMXRecord(record.Value)
		m.Answer = append(m.Answer, &dns.MX{
			Hdr:        header,
			Preference: priority,
			Mx:         dns.Fqdn(target),
		})
	case "NS":
		header.Rrtype = dns.TypeNS
		m.Answer = append(m.Answer, &dns.NS{
			Hdr: header,
			Ns:  dns.Fqdn(record.Value),
		})
	case "PTR":
		header.Rrtype = dns.TypePTR
		m.Answer = append(m.Answer, &dns.PTR{
			Hdr: header,
			Ptr: dns.Fqdn(record.Value),
		})
	}
}

// parseMXRecord parses an MX record value into priority and target
func (s *DNSServer) parseMXRecord(value string) (uint16, string) {
	parts := strings.Split(value, " ")
	priority := uint16(10) // Default priority
	target := parts[0]

	if len(parts) > 1 {
		if p, err := strconv.Atoi(parts[0]); err == nil {
			priority = uint16(p)
			target = parts[1]
		}
	}

	return priority, target
}

// handleUpstreamRequest forwards the request to an upstream DNS server
func (s *DNSServer) handleUpstreamRequest(w dns.ResponseWriter, r *dns.Msg) {
	response, err := s.forwardRequest(r)
	if err != nil {
		s.sendServerFailure(w, r, err)
		return
	}

	// Send the response
	w.WriteMsg(response)
}

// sendServerFailure sends a DNS server failure response
func (s *DNSServer) sendServerFailure(w dns.ResponseWriter, r *dns.Msg, err error) {
	log.Printf("Error handling DNS request: %v", err)
	m := new(dns.Msg)
	m.SetReply(r)
	m.SetRcode(r, dns.RcodeServerFailure)
	w.WriteMsg(m)
}

// forwardRequest forwards a DNS request to the appropriate upstream server
func (s *DNSServer) forwardRequest(r *dns.Msg) (*dns.Msg, error) {
	if len(r.Question) == 0 {
		return nil, fmt.Errorf("empty question section")
	}

	// Always use the first upstream for now
	// In a more advanced implementation, this is where routing logic would go
	var upstreamName string
	for name := range s.config.Upstreams {
		upstreamName = name
		break
	}

	upstream := s.config.Upstreams[upstreamName]
	client := s.upstreams[upstreamName]

	// Construct the address
	upstreamAddr := net.JoinHostPort(
		upstream.Address,
		strconv.Itoa(upstream.Port),
	)

	// Forward the request
	response, _, err := client.Exchange(r, upstreamAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to query upstream %s: %w", upstreamName, err)
	}

	return response, nil
}

// RouteByDomain would route DNS requests based on the domain name
// This is a placeholder for future implementation
func (s *DNSServer) routeByDomain(domain string) (string, error) {
	// This is where you would implement domain-based routing logic
	// For now, we'll just return the first upstream
	for name := range s.config.Upstreams {
		return name, nil
	}

	return "", fmt.Errorf("no suitable upstream found for domain: %s", domain)
}

// getDomainFromQuestion extracts the domain name from a DNS question
func getDomainFromQuestion(q dns.Question) string {
	return strings.TrimSuffix(q.Name, ".")
}
