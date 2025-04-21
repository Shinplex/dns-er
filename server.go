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
	// Log query if enabled
	if s.config.Server.LogQueries && len(r.Question) > 0 {
		q := r.Question[0]
		log.Printf("Query: %s, Type: %s", q.Name, dns.TypeToString[q.Qtype])
	}

	// Check if we have a local record for this query
	if len(r.Question) > 0 {
		q := r.Question[0]
		recordType := dns.TypeToString[q.Qtype]
		domain := getDomainFromQuestion(q)
		
		if record := FindMatchingRecord(domain, recordType); record != nil {
			// We have a matching record, return it
			m := new(dns.Msg)
			m.SetReply(r)
			
			switch recordType {
			case "A":
				m.Answer = append(m.Answer, &dns.A{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    uint32(record.TTL),
					},
					A: net.ParseIP(record.Value),
				})
			case "AAAA":
				m.Answer = append(m.Answer, &dns.AAAA{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    uint32(record.TTL),
					},
					AAAA: net.ParseIP(record.Value),
				})
			case "CNAME":
				m.Answer = append(m.Answer, &dns.CNAME{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeCNAME,
						Class:  dns.ClassINET,
						Ttl:    uint32(record.TTL),
					},
					Target: dns.Fqdn(record.Value),
				})
			case "TXT":
				m.Answer = append(m.Answer, &dns.TXT{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeTXT,
						Class:  dns.ClassINET,
						Ttl:    uint32(record.TTL),
					},
					Txt: []string{record.Value},
				})
			case "MX":
				parts := strings.Split(record.Value, " ")
				priority := uint16(10) // Default priority
				target := parts[0]
				
				if len(parts) > 1 {
					if p, err := strconv.Atoi(parts[0]); err == nil {
						priority = uint16(p)
						target = parts[1]
					}
				}
				
				m.Answer = append(m.Answer, &dns.MX{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeMX,
						Class:  dns.ClassINET,
						Ttl:    uint32(record.TTL),
					},
					Preference: priority,
					Mx:         dns.Fqdn(target),
				})
			case "NS":
				m.Answer = append(m.Answer, &dns.NS{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeNS,
						Class:  dns.ClassINET,
						Ttl:    uint32(record.TTL),
					},
					Ns: dns.Fqdn(record.Value),
				})
			case "PTR":
				m.Answer = append(m.Answer, &dns.PTR{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypePTR,
						Class:  dns.ClassINET,
						Ttl:    uint32(record.TTL),
					},
					Ptr: dns.Fqdn(record.Value),
				})
			}
			
			if len(m.Answer) > 0 {
				if s.config.Server.LogQueries {
					log.Printf("Response for %s from local records: %s", domain, recordType)
				}
				w.WriteMsg(m)
				return
			}
		}
	}

	// No matching local record, forward the request to the appropriate upstream server
	response, err := s.forwardRequest(r)
	if err != nil {
		log.Printf("Error forwarding request: %v", err)
		// Send a server failure response
		m := new(dns.Msg)
		m.SetReply(r)
		m.SetRcode(r, dns.RcodeServerFailure)
		w.WriteMsg(m)
		return
	}

	// Send the response
	w.WriteMsg(response)
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