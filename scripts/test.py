#!/usr/bin/env python3
"""
A simple interactive DNS query tool that uses a local DNS server.
This script uses only the Python standard library.
"""

import socket
import struct
import random
import sys
import time


class DNSQuery:
    def __init__(self):
        # Standard DNS header fields
        self.transaction_id = random.randint(0, 65535)
        self.flags = 0x0100  # Standard query with recursion desired
        self.questions = 1
        self.answer_rrs = 0
        self.authority_rrs = 0
        self.additional_rrs = 0
        self.qname = b""
        self.qtype = 1  # A record by default
        self.qclass = 1  # IN class

    def set_domain(self, domain):
        """Convert domain name to DNS format (length-prefixed labels)"""
        labels = domain.strip(".").split(".")
        result = b""
        for label in labels:
            result += struct.pack("B", len(label)) + label.encode("ascii")
        result += b"\x00"  # Terminate with zero length
        self.qname = result
        return self

    def set_type(self, qtype):
        """Set the query type (1=A, 28=AAAA, 5=CNAME, etc.)"""
        self.qtype = qtype
        return self

    def encode(self):
        """Encode the DNS query into a binary packet"""
        packet = struct.pack(
            "!HHHHHH",
            self.transaction_id,
            self.flags,
            self.questions,
            self.answer_rrs,
            self.authority_rrs,
            self.additional_rrs,
        )
        packet += self.qname
        packet += struct.pack("!HH", self.qtype, self.qclass)
        return packet


class DNSResponse:
    def __init__(self, data):
        self.data = data
        self.parse_header()
        self.answers = []
        self.parse_answers()

    def parse_header(self):
        header = struct.unpack("!HHHHHH", self.data[:12])
        self.transaction_id = header[0]
        self.flags = header[1]
        self.questions = header[2]
        self.answer_rrs = header[3]
        self.authority_rrs = header[4]
        self.additional_rrs = header[5]
        self.rcode = self.flags & 0x000F  # Response code

    def parse_name(self, offset):
        """Parse a name from the response, handling compression"""
        name = []
        original_offset = offset
        jumped = False
        max_jumps = 10  # Prevent infinite loops
        jumps = 0

        while True:
            length = self.data[offset]
            if (length & 0xC0) == 0xC0:  # Compression pointer
                if not jumped:
                    jumped = True
                    # Skip the pointer bytes for the next read
                    offset += 2
                
                # Follow the pointer
                pointer = ((length & 0x3F) << 8) | self.data[offset + 1]
                offset = pointer
                jumps += 1
                if jumps > max_jumps:
                    raise Exception("Too many jumps in DNS name")
                continue
            
            if length == 0:  # End of name
                offset += 1
                break
            
            offset += 1
            label = self.data[offset:offset + length].decode('ascii')
            name.append(label)
            offset += length
        
        if jumped:
            return ".".join(name), original_offset + 2
        else:
            return ".".join(name), offset

    def parse_answers(self):
        """Parse the answers section from the response"""
        offset = 12  # Skip the header
        
        # Skip the questions section
        for _ in range(self.questions):
            # Skip the query name
            while True:
                length = self.data[offset]
                if (length & 0xC0) == 0xC0:  # Compression pointer
                    offset += 2
                    break
                if length == 0:  # End of name
                    offset += 1
                    break
                offset += length + 1
            # Skip query type and class
            offset += 4
        
        # Parse answers
        for _ in range(self.answer_rrs):
            answer = {}
            
            # Parse name
            name, offset = self.parse_name(offset)
            answer["name"] = name
            
            # Parse type, class, TTL, and data length
            atype, aclass, ttl, data_len = struct.unpack("!HHIH", self.data[offset:offset+10])
            answer["type"] = atype
            answer["class"] = aclass
            answer["ttl"] = ttl
            answer["data_length"] = data_len
            offset += 10
            
            # Parse data based on record type
            if atype == 1:  # A record (IPv4)
                ip = ".".join(str(b) for b in self.data[offset:offset+4])
                answer["data"] = ip
            elif atype == 28:  # AAAA record (IPv6)
                ipv6_bytes = self.data[offset:offset+16]
                ipv6 = ":".join(format(ipv6_bytes[i:i+2].hex(), 'x') for i in range(0, 16, 2))
                answer["data"] = ipv6
            elif atype == 5:  # CNAME
                cname, _ = self.parse_name(offset)
                answer["data"] = cname
            elif atype == 2:  # NS
                ns, _ = self.parse_name(offset)
                answer["data"] = ns
            elif atype == 15:  # MX
                preference = struct.unpack("!H", self.data[offset:offset+2])[0]
                exchange, _ = self.parse_name(offset+2)
                answer["data"] = f"{preference} {exchange}"
            elif atype == 16:  # TXT
                txt_len = self.data[offset]
                txt_data = self.data[offset+1:offset+1+txt_len].decode('ascii', errors='replace')
                answer["data"] = txt_data
            else:
                answer["data"] = f"<Record type {atype}>"
            
            self.answers.append(answer)
            offset += data_len

    def __str__(self):
        """Format the DNS response as a string"""
        result = f"Transaction ID: {self.transaction_id}\n"
        result += f"Flags: 0x{self.flags:04x}\n"
        result += f"Response Code: {self.rcode}\n"
        result += f"Questions: {self.questions}\n"
        result += f"Answer RRs: {self.answer_rrs}\n"
        result += f"Authority RRs: {self.authority_rrs}\n"
        result += f"Additional RRs: {self.additional_rrs}\n\n"
        
        if self.rcode != 0:
            result += "Error Codes:\n"
            result += "  0: No error\n"
            result += "  1: Format error\n"
            result += "  2: Server failure\n"
            result += "  3: Name error (NXDOMAIN)\n"
            result += "  4: Not implemented\n"
            result += "  5: Refused\n"
            return result
        
        if not self.answers:
            result += "No answers found.\n"
            return result
        
        result += "Answers:\n"
        for i, answer in enumerate(self.answers, 1):
            result += f"Answer {i}:\n"
            result += f"  Name: {answer['name']}\n"
            
            # Translate record type to human-readable form
            type_names = {
                1: "A",
                2: "NS",
                5: "CNAME",
                6: "SOA",
                12: "PTR",
                15: "MX",
                16: "TXT",
                28: "AAAA",
            }
            type_name = type_names.get(answer['type'], str(answer['type']))
            
            result += f"  Type: {type_name}\n"
            result += f"  TTL: {answer['ttl']} seconds\n"
            result += f"  Data: {answer['data']}\n\n"
        
        return result


def query_dns(domain, qtype=1, server="127.0.0.1", port=53, timeout=5):
    """Send a DNS query to the specified server and return the response"""
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.settimeout(timeout)
    
    # Create and encode the query
    query = DNSQuery().set_domain(domain).set_type(qtype)
    packet = query.encode()
    
    try:
        # Send the query
        sock.sendto(packet, (server, port))
        
        # Receive the response
        response_data, _ = sock.recvfrom(1024)
        response = DNSResponse(response_data)
        return response
    except socket.timeout:
        print(f"Error: DNS query timed out after {timeout} seconds")
        return None
    except Exception as e:
        print(f"Error: {str(e)}")
        return None
    finally:
        sock.close()


def get_record_type():
    """Show a menu of DNS record types and get user selection"""
    print("\nSelect DNS record type:")
    print("1. A (IPv4 address)")
    print("2. NS (Name Server)")
    print("5. CNAME (Canonical Name)")
    print("15. MX (Mail Exchange)")
    print("16. TXT (Text record)")
    print("28. AAAA (IPv6 address)")
    print("255. ANY (All records)")
    
    while True:
        try:
            choice = input("Enter record type number (default: 1): ").strip()
            if not choice:
                return 1
            choice = int(choice)
            return choice
        except ValueError:
            print("Please enter a valid number.")


def interactive_mode():
    """Run the DNS query tool in interactive mode"""
    print("DNS Query Tool")
    print("Use Ctrl+C to exit.")
    
    # Default values
    default_server = "127.0.0.1"
    default_port = 53
    
    # Get server and port
    try:
        server = input(f"Enter DNS server IP (default: {default_server}): ").strip()
        if not server:
            server = default_server
        
        port_input = input(f"Enter DNS server port (default: {default_port}): ").strip()
        if not port_input:
            port = default_port
        else:
            port = int(port_input)
    except ValueError:
        print("Invalid port. Using default.")
        port = default_port
    
    while True:
        try:
            # Get the domain to query
            domain = input("\nEnter domain name to query: ").strip()
            if not domain:
                continue
            
            # Get the record type
            qtype = get_record_type()
            
            print(f"\nQuerying {domain} for record type {qtype} from {server}:{port}...")
            start_time = time.time()
            response = query_dns(domain, qtype, server, port)
            elapsed = (time.time() - start_time) * 1000  # Convert to milliseconds
            
            if response:
                print(f"Response received in {elapsed:.2f} ms:")
                print(response)
            
            cont = input("Make another query? (y/n): ").strip().lower()
            if cont != 'y':
                break
                
        except KeyboardInterrupt:
            print("\nExiting...")
            break
        except Exception as e:
            print(f"Error: {str(e)}")


if __name__ == "__main__":
    try:
        interactive_mode()
    except KeyboardInterrupt:
        print("\nExiting...")
        sys.exit(0)