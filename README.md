dhcp6d [![GoDoc](https://godoc.org/github.com/oiooj/dhcp6d?status.svg)](https://godoc.org/github.com/oiooj/dhcp6d) [![Go Report Card](https://goreportcard.com/badge/github.com/oiooj/dhcp6d)](https://goreportcard.com/report/github.com/oiooj/dhcp6d)
=====

Package `dhcp6d` implements a DHCPv6 server, as described in IETF RFC 3315.  MIT Licensed.

It's based on mdlayher's dhcp6.

At this time, the API is not stable, and may change over time.  The eventual
goal is to implement a server, client, and testing facilities for consumers
of this package.

The design of this package is inspired by Go's `net/http` package.  The Go
standard library is Copyright (c) 2018 The Go Authors. All rights reserved.
The Go license can be found at https://golang.org/LICENSE.
