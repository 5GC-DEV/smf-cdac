// SPDX-FileCopyrightText: 2022-present Intel Corporation
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"errors"
	"net"
	"os"
	"strconv"
	"sync"

	"github.com/omec-project/smf/logger"
)

type IPAllocator struct {
	ipNetwork *net.IPNet
	g         *_IDPool
}

func NewIPAllocator(cidr string) (*IPAllocator, error) {
	allocator := &IPAllocator{}

	if _, ipnet, err := net.ParseCIDR(cidr); err != nil {
		return nil, err
	} else {
		allocator.ipNetwork = ipnet
	}
	allocator.g = newIDPool(1, 1<<int64(32-maskBits(allocator.ipNetwork.Mask))-2)

	return allocator, nil
}

func maskBits(mask net.IPMask) int {
	var cnt int
	for _, b := range mask {
		for ; b != 0; b /= 2 {
			if b%2 != 0 {
				cnt++
			}
		}
	}
	return cnt
}

// IPAddrWithOffset add offset on base ip
func IPAddrWithOffset(ip net.IP, offset int) net.IP {
	retIP := make(net.IP, len(ip))
	copy(retIP, ip)
	logger.CtxLog.Infof("IPAddrWithOffset: Base IP: %s, Offset: %d", ip.String(), offset)
	var carry int
	for i := len(retIP) - 1; i >= 0; i-- {
		if offset == 0 {
			break
		}

		val := int(retIP[i]) + carry + offset%256
		retIP[i] = byte(val % 256)
		carry = val / 256
		logger.CtxLog.Debugf("IPAddrWithOffset: byte[%d] updated to %d, carry: %d, remaining offset: %d", i, retIP[i], carry, offset/256)

		offset /= 256
	}
	logger.CtxLog.Infof("IPAddrWithOffset RESULT >>> resultIP=%s remainingOffset=%d carry=%d", retIP.String(), offset, carry)
	logger.CtxLog.Infof("IPAddrWithOffset: Resulting IP: %s", retIP.String())
	return retIP
}

// IPAddrOffset calculate the input ip with base ip offset
func IPAddrOffset(in, base net.IP) int {
	offset := 0
	exp := 1
	for i := len(base) - 1; i >= 0; i-- {
		offset += int(in[i]-base[i]) * exp
		exp *= 256
	}
	return offset
}

// Allocate will allocate the IP address and returns it
func (a *IPAllocator) Allocate(imsi string) (net.IP, error) {
	// check if static IP already reserved for this IMSI
	if a.g.staticIps != nil {
		staticIps := *a.g.staticIps
		if ipStr := staticIps[imsi]; ipStr != "" {
			logger.CtxLog.Infof("IPAllocator: Static IP already reserved for IMSI %s: %s", imsi, net.ParseIP(ipStr).To4().String())
			return net.ParseIP(ipStr).To4(), nil
		}
	}

	if offset, err := a.g.allocate(); err != nil {
		logger.CtxLog.Errorf("IPAllocator: Failed to allocate IP for IMSI %s: %v", imsi, err)
		return nil, errors.New("ip allocation failed" + err.Error())
	} else {
		smfCountStr := os.Getenv("SMF_COUNT")
		if smfCountStr == "" {
			smfCountStr = "1"
		}
		smfCount, err := strconv.Atoi(smfCountStr)
		if err != nil {
			logger.CtxLog.Errorf("failed to convert SMF_COUNT to int: %v", err)
		}
		ip := IPAddrWithOffset(a.ipNetwork.IP, int(offset)) // int64((smfCount-1)*5000 + 1)
		logger.CtxLog.Infof("IPAllocator: Dynamic IP allocation successful for IMSI %s", imsi)
		logger.CtxLog.Infof("  Allocated IP: %s", ip.String())
		logger.CtxLog.Infof("unique id - ip %v", ip)
		logger.CtxLog.Infof("unique id - offset %v", offset)
		logger.CtxLog.Infof("unique id after allocate- smfCount %v", smfCount)
		if a.ipNetwork != nil {
			logger.CtxLog.Infof("  Base network IP: %s", a.ipNetwork.IP.String())
			logger.CtxLog.Infof("  IP Network Mask: %s", a.ipNetwork.Mask.String())
		}
		return ip, nil
	}
}

func (a *IPAllocator) ReserveStaticIps(ips *map[string]string) {
	a.g.staticIps = ips
	for _, ipStr := range *ips {
		if ip := net.ParseIP(ipStr).To4(); ip != nil {
			// block static IPs in pool to avoid dynamic allocation
			a.BlockIp(ip)
		}
	}
}

func (a *IPAllocator) BlockIp(ip net.IP) {
	offset := IPAddrOffset(ip, a.ipNetwork.IP)
	a.g.block(int64(offset))
}

func (a *IPAllocator) Release(imsi string, ip net.IP) {
	if a == nil || a.g == nil || a.ipNetwork == nil {
		logger.CtxLog.Errorf("IPAllocator not initialized properly, cannot release IP %s for IMSI %s", ip.String(), imsi)
	}
	logger.CtxLog.Debugf("Releasing IP %s for IMSI %s", ip.String(), imsi)
	// Don't release static IPs
	if a.g.staticIps != nil {
		staticIps := *a.g.staticIps
		if ipStr := staticIps[imsi]; ipStr != "" {
			logger.CtxLog.Debugf("IPAllocator: Not releasing static IP %s for IMSI %s", ipStr, imsi)
			return
		}
	}

	offset := IPAddrOffset(ip, a.ipNetwork.IP)
	logger.CtxLog.Debugf("IPAllocator: Calculated offset %d for IP %s", offset, ip.String())
	a.g.release(int64(offset))
	logger.CtxLog.Infof("IPAllocator: Released IP %s (offset %d) for IMSI %s", ip.String(), offset, imsi)
}

type _IDPool struct {
	staticIps *map[string]string // map of [imsi]ip
	isUsed    map[int64]bool
	minValue  int64
	maxValue  int64
	index     int64
	lock      sync.Mutex
}

func newIDPool(minValue int64, maxValue int64) (idPool *_IDPool) {
	idPool = new(_IDPool)
	idPool.minValue = minValue
	idPool.maxValue = maxValue
	idPool.isUsed = make(map[int64]bool)
	smfCountStr := os.Getenv("SMF_COUNT")
	if smfCountStr == "" {
		smfCountStr = "1"
	}
	smfCount, err := strconv.Atoi(smfCountStr)
	if err != nil {
		logger.CtxLog.Errorf("failed to convert SMF_COUNT to int: %v", err)
	}
	idPool.index = int64((smfCount-1)*2500 + 1)
	logger.CtxLog.Infof("IDPool INIT >>> minValue=%d maxValue=%d index=%d", minValue, maxValue, idPool.index)
	logger.CtxLog.Infof("IDPool INIT >>> smfCount=%d", smfCount)
	return
}

func (i *_IDPool) allocate() (id int64, err error) {
	i.lock.Lock()
	defer i.lock.Unlock()
	logger.CtxLog.Infof("IDPool ALLOCATE START >>> index=%d min=%d max=%d usedCount=%d", i.index, i.minValue, i.maxValue, len(i.isUsed))
	logger.CtxLog.Debugf("IDPool: Starting ID allocation from index %d to maxValue %d", i.index, i.maxValue)
	smfCountStr := os.Getenv("SMF_COUNT")
	if smfCountStr == "" {
		smfCountStr = "1"
	}
	smfCount, err := strconv.Atoi(smfCountStr)
	logger.CtxLog.Infof("IDPool ALLOCATE START >>> smfCount=%d", smfCount)
	if err != nil {
		logger.CtxLog.Errorf("failed to convert SMF_COUNT to int: %v", err)
	}
	for id = i.index; id <= i.maxValue; id++ {
		logger.CtxLog.Infof("IDPool LOOP >>> trying id=%d (index=%d max=%d)", id, i.index, i.maxValue)
		if _, exist := i.isUsed[id]; !exist {
			logger.CtxLog.Infof("IDPool LOOP >>> id=%d already in use, continue", id)
			i.isUsed[id] = true
			i.index = (id % i.maxValue) + 1
			if i.index == 1 {
				i.index = int64((smfCount-1)*2500 + 1)
			}
			logger.CtxLog.Infof("IDPool: Allocated ID %d (first loop), next index set to %d", id, i.index)
			return id, nil
		} else {
			logger.CtxLog.Infof("IDPool: ID %d already in use", id)
		}
	}

	logger.CtxLog.Debugf("IDPool: Wrapping around, checking IDs from 1 to %d", i.index-1)
	logger.CtxLog.Infof("IDPool WRAP >>> index reset from %d to minValue=%d", i.index, i.minValue)
	id_init := int64((smfCount-1)*2500 + 1)
	if id_init <= i.maxValue {
		for id = id_init; id <= i.index; id++ {
			logger.CtxLog.Infof("IDPool WRAP LOOP >>> trying id=%d", id)
			if _, exist := i.isUsed[id]; !exist {
				i.isUsed[id] = true
				i.index = id + 1
				logger.CtxLog.Infof("IDPool: Allocated ID %d (wrap-around), next index set to %d", id, i.index)
				return id, nil
			} else {
				logger.CtxLog.Infof("IDPool: ID %d already in use (wrap-around)", id)
			}
		}
	}
	logger.CtxLog.Infof("IDPool EXHAUSTED >>> no free IDs (used=%d maxValue=%d)", len(i.isUsed), i.maxValue)
	logger.CtxLog.Infof("IDPool: No available value range to allocate ID")
	return 0, errors.New("no available value range to allocate id")
}

func (i *_IDPool) block(id int64) {
	i.lock.Lock()
	defer i.lock.Unlock()
	i.isUsed[id] = true
}

func (i *_IDPool) release(id int64) {
	i.lock.Lock()
	defer i.lock.Unlock()
	delete(i.isUsed, id)
}
