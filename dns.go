// Copyright (c) 2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"log"
	"net"
	"qitmeer/core/protocol"
	"strconv"
	"strings"

	"github.com/miekg/dns"
)

type DNSServer struct {
	hostname   string
	listen     string
	nameserver string
}

func (d *DNSServer) Start() {
	defer wg.Done()

	rr := fmt.Sprintf("%s 86400 IN NS %s", d.hostname, d.nameserver)
	authority, err := dns.NewRR(rr)
	if err != nil {
		log.Printf("NewRR: %v", err)
		return
	}

	tcpAddr, err := net.ResolveTCPAddr("tcp4", d.listen)
	if err != nil {
		log.Printf("ResolveTCPAddr: %v", err)
		return
	}

	tcpListener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		log.Printf("ListenTCP: %v", err)
		return
	}
	//noinspection GoUnhandledErrorResult
	defer tcpListener.Close()

	for {
		conn, err := tcpListener.Accept()

		if err != nil {
			log.Printf("Accept: %v", err)
			continue
		}

		go func() {
			buffer := make([]byte, 512)

			n, err := conn.Read(buffer[0:])

			if err != nil {
				log.Printf("Read: %v", err)
				return
			}

			tcpAddr := conn.RemoteAddr()
			fmt.Println("Receive from client", tcpAddr.String(), string(buffer[0:n]))

			//noinspection GoUnhandledErrorResult
			defer conn.Close()

			dnsMsg := new(dns.Msg)
			err = dnsMsg.Unpack(buffer[:])
			if err != nil {
				log.Printf("%s: invalid dns message: %v",
					tcpAddr, err)
				return
			}
			if len(dnsMsg.Question) != 1 {
				log.Printf("%s sent more than 1 question: %d",
					tcpAddr, len(dnsMsg.Question))
				return
			}
			domainName := strings.ToLower(dnsMsg.Question[0].Name)
			ff := strings.LastIndex(domainName, d.hostname)
			if ff < 0 {
				log.Printf("invalid name: %s",
					dnsMsg.Question[0].Name)
				return
			}

			wantedSF := protocol.SFNodeNetwork
			labels := dns.SplitDomainName(domainName)
			if labels[0][0] == 'x' && len(labels[0]) > 1 {
				wantedSFStr := labels[0][1:]
				u, err := strconv.ParseUint(wantedSFStr, 10, 64)
				if err != nil {
					log.Printf("%s: ParseUint: %v", tcpAddr, err)
					return
				}
				wantedSF = protocol.ServiceFlag(u)
			}

			var atype string
			qtype := dnsMsg.Question[0].Qtype
			switch qtype {
			case dns.TypeA:
				atype = "A"
			case dns.TypeAAAA:
				atype = "AAAA"
			case dns.TypeNS:
				atype = "NS"
			default:
				log.Printf("%s: invalid qtype: %d", tcpAddr,
					dnsMsg.Question[0].Qtype)
				return
			}

			log.Printf("%s: query %d for %v", tcpAddr,
				dnsMsg.Question[0].Qtype, wantedSF)

			respMsg := dnsMsg.Copy()
			respMsg.Authoritative = true
			respMsg.Response = true

			if qtype != dns.TypeNS {
				respMsg.Ns = append(respMsg.Ns, authority)
				ips := amgr.GoodAddresses(qtype, wantedSF)
				for _, ip := range ips {
					rr = fmt.Sprintf("%s 30 IN %s %s",
						dnsMsg.Question[0].Name, atype,
						ip.String())
					newRR, err := dns.NewRR(rr)
					if err != nil {
						log.Printf("%s: NewRR: %v",
							tcpAddr, err)
						return
					}

					respMsg.Answer = append(respMsg.Answer,
						newRR)
				}
			} else {
				rr = fmt.Sprintf("%s 86400 IN NS %s",
					dnsMsg.Question[0].Name, d.nameserver)
				newRR, err := dns.NewRR(rr)
				if err != nil {
					log.Printf("%s: NewRR: %v", tcpAddr, err)
					return
				}

				respMsg.Answer = append(respMsg.Answer, newRR)
			}

			//done:
			sendBytes, err := respMsg.Pack()
			if err != nil {
				log.Printf("%s: failed to pack response: %v",
					tcpAddr, err)
				return
			}

			_, err = conn.Write(sendBytes)
			if err != nil {
				log.Printf("%s: failed to write response: %v",
					tcpAddr, err)
				return
			}
		}()

	}
}

func NewDNSServer(hostname, nameserver, listen string) *DNSServer {
	if hostname[len(hostname)-1] != '.' {
		hostname = hostname + "."
	}
	if nameserver[len(nameserver)-1] != '.' {
		nameserver = nameserver + "."
	}

	return &DNSServer{
		hostname:   hostname,
		listen:     listen,
		nameserver: nameserver,
	}
}
