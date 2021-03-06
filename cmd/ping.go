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
	"bytes"
	"errors"
	"io/ioutil"
	"os"
	"strings"
	"sync/atomic"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"

	"net"

	"time"

	"fmt"
	"strconv"

	ui "github.com/gizak/termui"

	"github.com/mythay/anet/util"
	"github.com/mythay/modbus"
	"github.com/spf13/cobra"
)

var flagPingCount, flagPingInterval, flagPingTimeout, flagPingRecoverySlot, flagPingPacketSize int
var flagPingForever bool
var pauseSignal = make(chan int)

var mbServer *modbus.TcpServer
var mbh mbhandler

const icmpPadPattern = "Henry Hei create this program for TeSysT. "

var icmpPad []byte

// pingCmd represents the ping command
var pingCmd = &cobra.Command{
	Use:   "ping [ip]...",
	Short: "Advanced ping command (need super privillege)",
	Long: `Advanced ping command to test multiple target at the sametime (need super privillege). 
For example:
	anet ping 192.168.1.1-10`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Work your own magic here
		var err error
		nargs := len(args)
		if nargs == 0 {
			return errors.New("at least one ip is needed")
		}
		if flagPingPacketSize < 0 || flagPingPacketSize > 1518 {
			return fmt.Errorf("invalid icmp packet size '%d'", flagPingPacketSize)
		}
		icmpPad = bytes.NewBufferString(strings.Repeat(icmpPadPattern, flagPingPacketSize/len(icmpPadPattern)+1)).Bytes()[:flagPingPacketSize]
		ips, err := util.ParseMultipleIPRange(args...)
		if err != nil {
			return err
		}
		mbServer, err = modbus.NewTcpServer(502)
		if err != nil {
			return err
		}

		mbh.data[0] = uint16(flagPingInterval)
		mbh.data[1] = uint16(flagPingTimeout)

		go func() {
			defer mbServer.Close()
			mbServer.ServeModbus(&mbh)
		}()
		err = ui.Init()
		if err != nil {
			panic(err)
		}
		defer ui.Close()

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
			p, err := newPinger(ip, time.Millisecond*time.Duration(flagPingInterval), time.Millisecond*time.Duration(flagPingTimeout), time.Millisecond*time.Duration(flagPingRecoverySlot))
			if err != nil {

				return err
			}
			pingers = append(pingers, p)
			spl := ui.NewSparkline()
			spl.Data = nil
			spl.Title = ip.String()
			spl.LineColor = ui.ColorGreen
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
		var clearStat = func() {
			for i, p := range pingers {
				p.resetCounter()
				sparkLines.Lines[i].Title = fmt.Sprintf("%-15s avg:%-4d max:%-4d err:%-4d/%-4d ", p.conn.RemoteAddr().String(), p.average/1e6, p.max/1e6, p.errcount, p.reqcount)
				sparkLines.Lines[i].Data = toms(p.rtt, 80)
				statBox.Rows[1][0] = ""
			}
		}
		var updateStat = func() {
			var maxRtt time.Duration
			var totalCount, errorCount, maxErrorCount uint32
			var maxRttIP, maxErrIP string

			for i, p := range pingers {
				if maxRtt < p.max {
					maxRtt = p.max
					maxRttIP = p.conn.RemoteAddr().String()
				}
				if maxErrorCount < p.errcount {
					maxErrorCount = p.errcount
					maxErrIP = p.conn.RemoteAddr().String()
				}
				errorCount += p.errcount
				totalCount += p.reqcount

				sparkLines.Lines[i].Title = fmt.Sprintf("%-15s avg:%-4d max:%-4d err:%-4d/%-4d | %v", p.conn.RemoteAddr().String(), p.average/1e6, p.max/1e6, p.errcount, p.reqcount, p.errlist)
				sparkLines.Lines[i].Data = toms(p.rtt, 80)
			}
			startTime := statBox.Rows[1][0]
			if startTime == "" {
				startTime = time.Now().Format("15:04:05")
			}
			statBox.Rows[1] = []string{startTime, strconv.FormatUint(uint64(totalCount), 10), strconv.FormatUint(uint64(errorCount), 10), strconv.FormatUint(uint64(maxRtt/1e6), 10), maxRttIP, maxErrIP}
			ui.Body.Align()
			ui.Clear()
			ui.Render(ui.Body)
		}

		ui.Handle("/timer/1s", func(ui.Event) {
			updateStat()
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
			clearStat()
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
	pingCmd.Flags().IntVarP(&flagPingPacketSize, "packetsize", "s", 22, "the number of data bytes to be sent")
	pingCmd.Flags().IntVarP(&flagPingInterval, "interval", "i", 1000, "interval ms between two request")
	pingCmd.Flags().IntVarP(&flagPingTimeout, "timeout", "W", 5000, "wait timeout ms for response")
	pingCmd.Flags().IntVarP(&flagPingRecoverySlot, "recover", "r", 0, "recover checking slot to reset errors")
	pingCmd.Flags().BoolVarP(&flagPingForever, "forever", "t", false, "pinging forever")

}

type pinger struct {
	conn         net.Conn
	seq          int
	sucesscount  uint32
	reqcount     uint32
	errcount     uint32
	errlist      []uint32
	islasterror  bool
	rtt          []time.Duration
	average      time.Duration
	max          time.Duration
	interval     time.Duration
	timeout      time.Duration
	recoverSlot  time.Duration
	recoverTimer *time.Timer
	mbaddress    int
}

func newPinger(ip net.IP, interval time.Duration, timeout time.Duration, recoverSlot time.Duration) (*pinger, error) {
	var err error
	p := &pinger{interval: interval, timeout: timeout, recoverSlot: recoverSlot, mbaddress: int(ip.To4()[3]) * 10}
	ip.To4()
	if recoverSlot > 0 {
		p.recoverTimer = time.AfterFunc(recoverSlot, p.pushErrBacklog)
	}
	p.conn, err = net.DialTimeout("ip4:icmp", ip.String(), time.Second)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (p *pinger) pushErrBacklog() {
	count := atomic.LoadUint32(&p.errcount)
	atomic.StoreUint32(&p.errcount, 0)
	if count > 0 {
		p.errlist = append(p.errlist, count)
	}
}

func (p *pinger) once() error {
	var err error
	p.seq++
	if p.seq > 0xffff {
		p.seq = 0
	}
	reqPacket, _ := p.marshalMsg(icmpPad)

	p.reqcount++

	start := time.Now()
	p.conn.SetDeadline(start.Add(p.timeout))
	if _, err = p.conn.Write(reqPacket); err != nil {
		return err
	}

	respPacket := make([]byte, 1500)
	n, err := p.conn.Read(respPacket)
	if err != nil {
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
		return err
	}
	if rm.Type == ipv4.ICMPTypeEchoReply {
		body := rm.Body.(*icmp.Echo)
		if body.Seq != p.seq {
			err = fmt.Errorf("sequence not equal, expect %d, but %d", p.seq, body.Seq)
			return err
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
		return fmt.Errorf("invalid icmp response")
	}
	return nil
}

func (p *pinger) ping(count int) {
	var oneloop = func() {
		_, _ = <-pauseSignal
		start := time.Now()
		err := p.once()
		if err != nil {
			flush(p.conn, start.Add(p.interval))
			atomic.AddUint32(&p.errcount, 1)
			p.errcount++
			p.islasterror = true
			// fmt.Println(err)
		} else {
			if p.islasterror && p.recoverTimer != nil {
				p.islasterror = false
				p.recoverTimer.Reset(p.recoverSlot)
			}
		}

		mbh.data[p.mbaddress] = uint16(p.errcount)
		mbh.data[p.mbaddress+1] = uint16(p.max / 1e6)
		mbh.data[p.mbaddress+2] = uint16(p.reqcount)
		for index, value := range p.errlist {
			mbh.data[p.mbaddress+index+3] = uint16(value)
		}

		duration := time.Now().Sub(start)
		if duration < p.interval {
			time.Sleep(p.interval - duration)
		}
	}
	if count < 0 {
		for {
			oneloop()
		}
	} else {
		for i := 0; i < count; i++ {
			oneloop()
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

func (p *pinger) resetCounter() {
	p.average = 0
	p.max = 0
	p.errcount = 0
	p.sucesscount = 0
	p.reqcount = 0
	// p.seq = 0
	p.rtt = []time.Duration{}
	p.errlist = []uint32{}
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

type mbhandler struct {
	data [256 * 10]uint16
}

func (s *mbhandler) ReadHoldingRegisters(slaveid byte, address, quantity uint16) ([]uint16, error) {
	var buf []uint16
	if address < uint16(len(s.data)) && address+quantity-1 < uint16(len(s.data)) {
		buf = append(buf, s.data[address:address+quantity]...)
		return buf, nil
	}
	return nil, fmt.Errorf("out of range")
}
func (s *mbhandler) WriteSingleRegister(slaveid byte, address, value uint16) error {
	if address < uint16(len(s.data)) {
		s.data[address] = value
		if s.data[9] > 0 { // monitor the register 9 to clear
			s.data[9] = 0
			ui.SendCustomEvt("/sys/kbd/c", nil)
		}
		return nil
	}
	return fmt.Errorf("out of range")
}

func flush(c net.Conn, t time.Time) (err error) {
	if err = c.SetReadDeadline(t); err != nil {
		return
	}
	// Timeout setting will be reset when reading
	if _, err = ioutil.ReadAll(c); err != nil {
		// Ignore timeout error
		if netError, ok := err.(net.Error); ok && netError.Timeout() {
			err = nil
		}
	}
	return
}
