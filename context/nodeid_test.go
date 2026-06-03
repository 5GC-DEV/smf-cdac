// SPDX-License-Identifier: Apache-2.0
// Copyright 2024 Canonical Ltd.

package context_test

import (
	"net"
	"testing"

	"github.com/omec-project/smf/context"
)

const (
	errNodeIDTypeMismatchFmt = "expected NodeIdType to be %d, got %d"
	errExpectedIPv4Fmt       = "expected 1.2.3.4 got %v"
	defaultIPv4Addr          = "1.2.3.4"
	defaultIPv6Addr          = "2001:db8::68"
)

func TestNewNodeIDIpv4(t *testing.T) {
	nodeID := context.NewNodeID(defaultIPv4Addr)

	if nodeID.NodeIdType != context.NodeIdTypeIpv4Address {
		t.Errorf(errNodeIDTypeMismatchFmt, context.NodeIdTypeIpv4Address, nodeID.NodeIdType)
	}

	if net.IP(nodeID.NodeIdValue).String() != net.ParseIP(defaultIPv4Addr).String() {
		t.Errorf(errExpectedIPv4Fmt, net.IP(nodeID.NodeIdValue))
	}
}

func TestNewNodeIDIpv6(t *testing.T) {
	nodeID := context.NewNodeID(defaultIPv6Addr)

	if nodeID.NodeIdType != context.NodeIdTypeIpv6Address {
		t.Errorf(errNodeIDTypeMismatchFmt, context.NodeIdTypeIpv6Address, nodeID.NodeIdType)
	}

	if net.IP(nodeID.NodeIdValue).String() != net.ParseIP(defaultIPv6Addr).String() {
		t.Errorf("expected 2001:db8::68 got %v", net.IP(nodeID.NodeIdValue))
	}
}

func TestNewNodeIDFqdn(t *testing.T) {
	nodeID := context.NewNodeID("example.com")

	if nodeID.NodeIdType != context.NodeIdTypeFqdn {
		t.Errorf(errNodeIDTypeMismatchFmt, context.NodeIdTypeFqdn, nodeID.NodeIdType)
	}

	if string(nodeID.NodeIdValue) != "example.com" {
		t.Errorf("expected example.com got %s", nodeID.NodeIdValue)
	}
}

func TestResolveNodeIdToIpForIpv4(t *testing.T) {
	nodeID := context.NewNodeID(defaultIPv4Addr)

	if nodeID.ResolveNodeIdToIp().String() != net.ParseIP(defaultIPv4Addr).String() {
		t.Errorf(errExpectedIPv4Fmt, nodeID.ResolveNodeIdToIp())
	}
}

func TestResolveNodeIdToIpForIpv6(t *testing.T) {
	nodeID := context.NewNodeID(defaultIPv6Addr)

	if nodeID.ResolveNodeIdToIp().String() != net.ParseIP(defaultIPv6Addr).String() {
		t.Errorf("expected 2001:db8::68 got %v", nodeID.ResolveNodeIdToIp())
	}
}

func TestResolveNodeIdToIpForFqdn(t *testing.T) {
	context.InsertDnsHostIp("test.com", net.ParseIP(defaultIPv4Addr))
	nodeID := context.NewNodeID("test.com")

	ip := nodeID.ResolveNodeIdToIp()

	if ip.String() != net.ParseIP(defaultIPv4Addr).String() {
		t.Errorf(errExpectedIPv4Fmt, ip)
	}
}
