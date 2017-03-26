package anet

import (
	"fmt"
	"net"
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
