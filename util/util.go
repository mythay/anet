package util

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
)

//ParseIPRange to parse ip adress in range types
// ips,err:=ParseIPRange("192.168.1.1-244")
func ParseIPRange(orig string) ([]net.IP, error) {
	if ip := net.ParseIP(orig); ip != nil {
		return []net.IP{ip}, nil
	}
	if parts := strings.Split(orig, "-"); len(parts) == 2 && net.ParseIP(parts[0]) != nil {
		startip := net.ParseIP(parts[0]).To4()
		end, err := strconv.Atoi(parts[1])
		if err == nil {
			start := int(startip[3])
			if end > start && end < 255 {
				ips := make([]net.IP, end-start+1)
				for i := 0; i < len(ips); i++ {
					ips[i] = net.IPv4(startip[0], startip[1], startip[2], byte(start+i))
				}
				return ips, nil
			}
		}
		return nil, fmt.Errorf("'%s' invalid ip range", orig)
	}
	return nil, fmt.Errorf("'%s' invalid ip format", orig)

}

//ParseMultipleIPRange to parse multiple ip adress in range types
// ips,err:=ParseIPRange("192.168.1.1-244")
func ParseMultipleIPRange(ipstrs ...string) ([]net.IP, error) {
	uniqIpmap := make(map[string]net.IP)
	for i, ipstr := range ipstrs {
		ips, err := ParseIPRange(ipstr)
		if err != nil {
			return nil, fmt.Errorf("ip range %d, %s", i, err.Error())
		}
		for _, ip := range ips {
			uniqIpmap[ip.String()] = ip
		}
	}
	var uniqIps []net.IP
	for _, v := range uniqIpmap {
		uniqIps = append(uniqIps, v)
	}
	sort.Sort(sortByIP(uniqIps))
	return uniqIps, nil

}

type sortByIP []net.IP

func (ips sortByIP) Len() int {
	return len(ips)
}

func (ips sortByIP) Swap(i, j int) {
	ips[i], ips[j] = ips[j], ips[i]
}

func (ips sortByIP) Less(i, j int) bool {
	a := ips[i].To4()
	b := ips[j].To4()
	return a[0] < b[0] || a[1] < b[1] || a[2] < b[2] || a[3] < b[3]
}
