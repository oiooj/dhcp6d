// Command dhcp6d is an example DHCPv6 dhcp6server.  It can only assign a
// single IPv6 address, and is not a complete DHCPv6 server implementation
// by any means.  It is meant to demonstrate usage of package dhcp6.
package main

import (
	"encoding/hex"
	"flag"
	"log"
	"net"
	"time"

	"github.com/mdlayher/eui64"
	"github.com/oiooj/dhcp6d"
	"github.com/oiooj/dhcp6d/dhcp6opts"
	"github.com/oiooj/dhcp6d/dhcp6server"
)

func main() {
	iface := flag.String("i", "eth0", "interface to serve DHCPv6")
	ipFlag := flag.String("subnet", "dead:beef:2018::/64", "IPv6 range addresses to serve over DHCPv6")
	flag.Parse()

	// Only accept a single IPv6 address
	ip, _, err := net.ParseCIDR(*ipFlag)
	if err != nil || ip == nil || ip.To4() != nil {
		log.Fatal("IP is not an IPv6 address")
	}

	// Make Handler to assign ip and use handle for requests
	h := &Handler{
		ip:      ip,
		handler: handle,
	}

	// Bind DHCPv6 server to interface and use specified handler
	log.Printf("binding DHCPv6 server to interface %s...", *iface)
	if err := dhcp6server.ListenAndServe(*iface, h); err != nil {
		log.Fatal(err)
	}
}

// A Handler is a basic DHCPv6 handler.
type Handler struct {
	ip      net.IP
	handler handler
}

// ServeDHCP is a dhcp6.Handler which invokes an internal handler that
// allows errors to be returned and handled in one place.
func (h *Handler) ServeDHCP(w dhcp6server.ResponseSender, r *dhcp6server.Request) {
	if err := h.handler(h.ip, w, r); err != nil {
		log.Println(err)
	}
}

// A handler is a DHCPv6 handler function which can assign a single IPv6
// address and also return an error.
type handler func(ip net.IP, w dhcp6server.ResponseSender, r *dhcp6server.Request) error

// handle is a handler which assigns IPv6 addresses using DHCPv6.
func handle(ip net.IP, w dhcp6server.ResponseSender, r *dhcp6server.Request) error {
	// Accept only Solicit, Request, or Confirm, since this server
	// does not handle Information Request or other message types
	valid := map[dhcp6.MessageType]handler{
		dhcp6.MessageTypeSolicit: solicitHandler,
		dhcp6.MessageTypeRequest: rHandler,
		dhcp6.MessageTypeConfirm: rHandler,
		dhcp6.MessageTypeRelease: releaseHandler,
		dhcp6.MessageTypeRenew:   rHandler,
		dhcp6.MessageTypeRebind:  rHandler,
	}
	h, ok := valid[r.MessageType]
	if !ok {
		log.Printf("MessageType： %s", r.MessageType)
		return nil
	}
	log.Printf("MessageType： %s", r.MessageType)

	// Make sure client sent a client ID.
	duid, err := r.Options.GetOne(dhcp6.OptionClientID)
	if err != nil {
		log.Printf("client ID not found")
		return nil
	}

	duidllt := new(dhcp6opts.DUIDLLT)
	duidll := new(dhcp6opts.DUIDLL)
	var mac net.HardwareAddr
	if err := duidllt.UnmarshalBinary(duid); err != nil {
		if err := duidll.UnmarshalBinary(duid); err != nil {
			log.Printf("unknown duid type")
			return nil
		} else {
			mac = duidll.HardwareAddr
		}
	} else {
		mac = duidllt.HardwareAddr
	}
	prefix, _, err := eui64.ParseIP(ip)
	if err != nil {
		log.Printf(err.Error())
		return err
	}

	ip, err = eui64.ParseMAC(prefix, mac)
	if err != nil {
		log.Printf(err.Error())
		return err
	}

	// Log information about the incoming request.
	log.Printf("[%s] ipv6: %s mac: %s remote: %s, type: %d, len: %d, tx: %s",
		hex.EncodeToString(duid),
		ip.To16(),
		mac.String(),
		r.RemoteAddr,
		r.MessageType,
		r.Length,
		hex.EncodeToString(r.TransactionID[:]),
	)

	// Print out options the client has requested
	if opts, err := dhcp6opts.GetOptionRequest(r.Options); err == nil {
		log.Println("- requested:")
		for _, o := range opts {
			log.Printf("\t - %s", o)
		}
	}
	return h(ip, w, r)
}

func releaseHandler(ip net.IP, w dhcp6server.ResponseSender, r *dhcp6server.Request) error {
	_, err := w.Send(dhcp6.MessageTypeReply)
	return err
}

func rHandler(ip net.IP, w dhcp6server.ResponseSender, r *dhcp6server.Request) error {
	// Client must send a IANA to retrieve an IPv6 address
	ianas, err := dhcp6opts.GetIANA(r.Options)
	if err == dhcp6.ErrOptionNotPresent {
		log.Println("no IANAs provided")
		return nil
	}
	if err != nil {
		return err
	}

	// Only accept one IANA
	if len(ianas) > 1 {
		log.Println("can only handle one IANA")
		return nil
	}
	ia := ianas[0]
	// update old IPv6
	iaaddr, err := dhcp6opts.NewIAAddr(ip, 60*time.Second, 90*time.Second, nil)
	if err != nil {
		return err
	}
	_ = ia.Options.Add(dhcp6.OptionIAAddr, iaaddr)
	_ = w.Options().Add(dhcp6.OptionIANA, ia)

	// Send reply to client
	_, err = w.Send(dhcp6.MessageTypeReply)
	return err
}

func solicitHandler(ip net.IP, w dhcp6server.ResponseSender, r *dhcp6server.Request) error {
	// Client must send a IANA to retrieve an IPv6 address
	ianas, err := dhcp6opts.GetIANA(r.Options)
	if err == dhcp6.ErrOptionNotPresent {
		log.Println("no IANAs provided")
		return nil
	}
	if err != nil {
		return err
	}

	// Only accept one IANA
	if len(ianas) > 1 {
		log.Println("can only handle one IANA")
		return nil
	}
	ia := ianas[0]

	log.Printf("IANA: %s (%s, %s)",
		hex.EncodeToString(ia.IAID[:]),
		ia.T1,
		ia.T2,
	)

	// Instruct client to prefer this server unconditionally
	_ = w.Options().Add(dhcp6.OptionPreference, dhcp6opts.Preference(255))
	return newIAAddr(ia, ip, w, r)
}

// newIAAddr creates a IAAddr for a IANA using the specified IPv6 address,
// and advertises it to a client.
func newIAAddr(ia *dhcp6opts.IANA, ip net.IP, w dhcp6server.ResponseSender, r *dhcp6server.Request) error {
	// Send IPv6 address with 60 second preferred lifetime,
	// 90 second valid lifetime, no extra options
	iaaddr, err := dhcp6opts.NewIAAddr(ip, 60*time, 90*time.Second, nil)
	if err != nil {
		return err
	}

	// Add IAAddr inside IANA, add IANA to options
	_ = ia.Options.Add(dhcp6.OptionIAAddr, iaaddr)
	_ = w.Options().Add(dhcp6.OptionIANA, ia)

	// Advertise address to soliciting clients
	log.Printf("advertising IP: %s", ip)
	_, err = w.Send(dhcp6.MessageTypeAdvertise)
	return err
}
