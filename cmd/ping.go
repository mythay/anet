// Copyright © 2017 NAME HERE <EMAIL ADDRESS>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"errors"
	"os"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"

	"net"

	"time"

	"fmt"
	"strconv"

	ui "github.com/gizak/termui"

	"github.com/mythay/anet/util"
	"github.com/spf13/cobra"
)

var flagPingCount, flagPingInterval, flagPingTimeout int
var flagPingForever bool
var pauseSignal = make(chan int)

// pingCmd represents the ping command
var pingCmd = &cobra.Command{
	Use:   "ping",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Work your own magic here
		err := ui.Init()
		if err != nil {
			panic(err)
		}
		defer ui.Close()
		nargs := len(args)
		if nargs == 0 {
			return errors.New("at least one ip is needed")
		}
		ips, err := util.ParseMultipleIPRange(args...)
		if err != nil {
			return err
		}
		// nips := len(ips)
		helpBox := ui.NewList()
		helpBox.Items = []string{
			"Press q to quit",
			"Press c to clear",
			"Press p to pause/continue",
		}
		helpBox.BorderLabel = "Help"
		helpBox.Height = 5
		helpBox.ItemFgColor = ui.ColorYellow
		statBox := ui.NewTable()
		statBox.Rows = [][]string{
			[]string{"time", "total", "error", "max rtt", "max rtt ip", "max err ip"},
			[]string{"", "0", "0", "0", "", ""},
		}
		statBox.Height = 5
		// statBox.Separator = false
		ui.Body.AddRows(ui.NewRow(ui.NewCol(8, 0, statBox), ui.NewCol(4, 0, helpBox)))
		var pingers []*pinger
		sparkLines := ui.NewSparklines()
		sparkLines.BorderLabel = "Dashboard"
		for _, ip := range ips {
			p, err := newPinger(ip, time.Millisecond*time.Duration(flagPingInterval), time.Millisecond*time.Duration(flagPingTimeout))
			if err != nil {
				return err
			}
			pingers = append(pingers, p)
			spl := ui.NewSparkline()
			spl.Data = nil
			spl.Title = ip.String()
			spl.LineColor = ui.ColorRed
			sparkLines.Add(spl)
		}
		sparkLines.Height = len(sparkLines.Lines)*2 + 2
		ui.Body.AddRows(ui.NewRow(ui.NewCol(10, 0, sparkLines)))

		for _, p := range pingers {
			go func(p *pinger) {
				if flagPingForever {
					p.ping(-1)
				} else {
					p.ping(flagPingCount)
				}

				// fmt.Println(p.conn.RemoteAddr(), p)
			}(p)
		}
		ui.Body.Align()

		ui.Render(ui.Body)

		ui.Handle("/timer/1s", func(ui.Event) {
			var maxRtt time.Duration
			var totalCount, errorCount, maxErrorCount int
			var maxRttIp, maxErrIp string

			for i, p := range pingers {
				if maxRtt < p.max {
					maxRtt = p.max
					maxRttIp = p.conn.RemoteAddr().String()
				}
				if maxErrorCount < p.errcount {
					maxErrorCount = p.errcount
					maxErrIp = p.conn.RemoteAddr().String()
				}
				errorCount += p.errcount
				totalCount += p.reqcount
				sparkLines.Lines[i].Title = fmt.Sprintf("%-15s avg:%-4d max:%-4d err:%-4d/%-4d", p.conn.RemoteAddr().String(), p.average/1e6, p.max/1e6, p.errcount, p.reqcount)
				sparkLines.Lines[i].Data = toms(p.rtt, 80)
			}
			startTime := statBox.Rows[1][0]
			if startTime == "" {
				startTime = time.Now().Format("15:04:05")
			}
			statBox.Rows[1] = []string{startTime, strconv.Itoa(totalCount), strconv.Itoa(errorCount), strconv.Itoa(int(maxRtt / 1e6)), maxRttIp, maxErrIp}
			ui.Body.Align()
			ui.Clear()
			ui.Render(ui.Body)
		})
		ui.Handle("/sys/kbd/q", func(ui.Event) {
			ui.StopLoop()
		})
		var pauseFlag = false
		close(pauseSignal)
		ui.Handle("/sys/kbd/p", func(ui.Event) {
			if !pauseFlag {
				pauseSignal = make(chan int)
			} else {
				close(pauseSignal)
			}
			pauseFlag = !pauseFlag
		})
		ui.Handle("/sys/kbd/c", func(ui.Event) {
			for i, p := range pingers {
				p.average = 0
				p.max = 0
				p.errcount = 0
				p.sucesscount = 0
				p.reqcount = 0
				p.seq = 0
				p.rtt = []time.Duration{}
				sparkLines.Lines[i].Title = fmt.Sprintf("%-15s avg:%-4d max:%-4d err:%-4d/%-4d", p.conn.RemoteAddr().String(), p.average/1e6, p.max/1e6, p.errcount, p.reqcount)
				sparkLines.Lines[i].Data = toms(p.rtt, 80)
				statBox.Rows[1][0] = ""
			}
		})
		ui.Handle("/sys/wnd/resize", func(e ui.Event) {
			ui.Body.Width = ui.TermWidth()
			ui.Body.Align()
			ui.Clear()
			ui.Render(ui.Body)
		})
		ui.Loop()
		return err
	},
}

func init() {
	RootCmd.AddCommand(pingCmd)
	pingCmd.Flags().IntVarP(&flagPingCount, "count", "c", 5, "pinging count until stop")
	pingCmd.Flags().IntVarP(&flagPingInterval, "interval", "i", 1000, "wait interval ms between sending two request")
	pingCmd.Flags().IntVarP(&flagPingTimeout, "timeout", "W", 5000, "wait timeout ms for response")
	pingCmd.Flags().BoolVarP(&flagPingForever, "forever", "t", false, "pinging forever")

}

type pinger struct {
	conn        net.Conn
	seq         int
	sucesscount int
	reqcount    int
	errcount    int
	rtt         []time.Duration
	average     time.Duration
	max         time.Duration
	interval    time.Duration
	timeout     time.Duration
}

func newPinger(ip net.IP, interval time.Duration, timeout time.Duration) (*pinger, error) {
	var err error
	p := &pinger{interval: interval, timeout: timeout}
	p.conn, err = net.DialTimeout("ip4:icmp", ip.String(), time.Second)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (p *pinger) once() error {
	p.seq++
	if p.seq > 0xffff {
		p.seq = 0
	}
	reqPacket, _ := p.marshalMsg(nil)
	p.reqcount++
	start := time.Now()
	p.conn.SetDeadline(start.Add(p.timeout))
	if _, err := p.conn.Write(reqPacket); err != nil {
		p.errcount++
		return err
	}

	respPacket := make([]byte, 1500)
	n, err := p.conn.Read(respPacket)
	if err != nil {
		p.errcount++
		return err
	}
	duration := time.Now().Sub(start)
	respPacket = func(b []byte) []byte {
		if len(b) < 20 {
			return b
		}
		hdrlen := int(b[0]&0x0f) << 2
		return b[hdrlen:]
	}(respPacket)
	rm, err := icmp.ParseMessage(1, respPacket[:n])
	if err != nil {
		p.errcount++
		return err
	}
	if rm.Type == ipv4.ICMPTypeEchoReply {
		body := rm.Body.(*icmp.Echo)
		if body.Seq != p.seq {
			p.errcount++
			return fmt.Errorf("sequence not equal, expect %d, but %d", p.seq, body.Seq)
		}
		if duration > p.max {
			p.max = duration
		}
		p.average = (p.average*time.Duration(p.sucesscount) + duration) / time.Duration(p.sucesscount+1)
		p.sucesscount++
		if len(p.rtt) > 1000 {
			p.rtt = append([]time.Duration{}, p.rtt[500:]...)
		}
		p.rtt = append(p.rtt, duration)

	} else {
		p.errcount++
	}
	return nil
}

func (p *pinger) ping(count int) {
	if count < 0 {
		for {
			_, _ = <-pauseSignal
			err := p.once()
			if err != nil {
				// fmt.Println(err)
			}
			time.Sleep(p.interval)
		}
	} else {
		for i := 0; i < count; i++ {
			_, _ = <-pauseSignal
			err := p.once()
			if err != nil {
				// fmt.Println(err)
			}
			time.Sleep(p.interval)
		}
	}

}

func (p *pinger) marshalMsg(data []byte) ([]byte, error) {
	xid, xseq := os.Getpid()&0xffff, p.seq
	req := icmp.Message{
		Type: ipv4.ICMPTypeEcho, Code: 0,
		Body: &icmp.Echo{
			ID: xid, Seq: xseq,
			Data: data,
		},
	}
	return req.Marshal(nil)
}

func toms(s []time.Duration, size int) []int {

	if size < len(s) {
		s = s[len(s)-size:]
	}
	r := make([]int, len(s))
	for i, t := range s {
		r[i] = int(t / 1000)
	}

	return r
}
