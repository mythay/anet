package cmd

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/mythay/anet/util"
)

func Test_newPinger(t *testing.T) {

	t.Run("ping for ever", func(t *testing.T) {
		fmt.Println("aaa")
		ips, _ := util.ParseMultipleIPRange("192.168.16.9-20")
		for _, ip := range ips {
			go func(ip net.IP) {
				p, _ := newPinger(ip, time.Millisecond*100, time.Millisecond*200)
				p.ping(-1)
			}(ip)
		}
		time.Sleep(time.Second * 20)

	})

}
