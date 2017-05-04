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
	"fmt"
	"os"
	"sync"

	"github.com/gosuri/uiprogress"
	"github.com/spf13/cobra"

	"net"

	"time"

	"github.com/dutchcoders/goftp"
	ui "github.com/gizak/termui"
	"github.com/mythay/anet/util"
)

var fUser, fPasswd, fDestdir, fLocalfile string
var flagGraph bool

// mputCmd represents the put command
var mputCmd = &cobra.Command{
	Use:   "mput [ip]...",
	Short: "upload file to ftp server",
	Long: `upload file to multiple ftp server at the same time.
For example:
	anet mput 192.168.1.1-10`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Work your own magic here
		nargs := len(args)
		if nargs == 0 {
			return errors.New("at least one ip is needed")
		}
		ips, err := util.ParseMultipleIPRange(args...)
		if err != nil {
			return err
		}

		if _, err := os.Stat(fLocalfile); err != nil {
			return fmt.Errorf("local file '%s' not exist", fLocalfile)
		}
		if flagGraph {
			return uploadDashView(ips)
		}
		return uploadSimpleView(ips)

	},
}

func init() {
	RootCmd.AddCommand(mputCmd)
	mputCmd.Flags().StringVarP(&fUser, "username", "u", "pcfactory", "A valid ftp user name")
	mputCmd.Flags().StringVarP(&fPasswd, "password", "p", "pcfactory", "Correspoding ftp password")
	mputCmd.Flags().StringVarP(&fDestdir, "directory", "d", "/fw", "Ftp server directory to store the file")
	mputCmd.Flags().StringVarP(&fLocalfile, "localfile", "l", "App2.out", "local file path to be uploaded")
	mputCmd.Flags().BoolVarP(&flagGraph, "graph", "g", false, "graph ui output")

}

type ftpServerInfo struct {
	ipaddr net.IP
	user   string
	passwd string
}

type fileUpload struct {
	fs      *os.File
	server  *ftpServerInfo
	destdir string
	offset  int
	size    int
	err     error
	status  string
	lock    sync.Mutex
}

func uploadSimpleView(ips []net.IP) error {
	// try to create all the task
	var progress = uiprogress.New()
	progress.Width = 30
	nips := len(ips)
	allprogress := make([]simpleUIBind, nips)
	wg := sync.WaitGroup{}
	wg.Add(nips)
	errmsg := make(chan error, nips)
	for i := range allprogress {
		allprogress[i].upload, _ = newFileUpload(&ftpServerInfo{
			user:   fUser,
			passwd: fPasswd,
			ipaddr: ips[i]},
			fLocalfile,
			fDestdir)
		allprogress[i].bar = progress.AddBar(100).AppendCompleted().PrependElapsed()
		allprogress[i].bar.PrependFunc(func(f *fileUpload) uiprogress.DecoratorFunc {
			return func(b *uiprogress.Bar) string {
				if f == nil {
					return fmt.Sprintf("%-15s :OPEN :ERROR", f.server.ipaddr.String())
				}

				if f.err != nil {
					return fmt.Sprintf("%-15s :%-8s :ERROR", f.server.ipaddr.String(), f.status)
				}
				if b.Current() != 100 {
					if f.size == 0 { // empty file,
						if f.status == "DONE" {
							b.Set(99)
							b.Incr()
						}

					} else {
						percent := f.offset * 100 / f.size
						b.Set(percent - 1)
						b.Incr()
					}
				}
				return fmt.Sprintf("%-15s :%-8s       ", f.server.ipaddr.String(), f.status)

			}
		}(allprogress[i].upload))
	}
	fmt.Println(len(allprogress))
	progress.Start()

	for _, item := range allprogress {
		go func(s simpleUIBind) {
			var err error
			defer wg.Done()
			err = s.upload.upload()
			if err != nil {
				errmsg <- err
			}
		}(item)
		time.Sleep(time.Millisecond * 10)
	}

	wg.Wait()
	time.Sleep(time.Millisecond * 200)
	progress.Stop()
	errcount := 0

	close(errmsg)
	for _ = range errmsg {
		errcount++
	}

	fmt.Printf("\nSTATISTIC: %d/%d success\n", nips-errcount, nips)
	return nil
}

func uploadDashView(ips []net.IP) error {
	err := ui.Init()
	if err != nil {
		panic(err)
	}
	defer ui.Close()

	nips := len(ips)
	allprogress := make([]dashUIBind, nips)

	sucessCount := 0
	errcount := 0
	quitLabel := ui.NewPar("Press q to exit")
	quitLabel.Border = false
	summaryLabel := ui.NewPar("Summary")
	summaryLabel.Border = false
	summaryPar := ui.NewPar(fmt.Sprintf(" %d/%d", sucessCount, nips))
	summaryPar.Border = false
	failLabel := ui.NewPar("Fail")
	failLabel.Border = false
	failPar := ui.NewPar(fmt.Sprintf(" %d", errcount))
	failPar.Border = false
	ui.Body.AddRows(ui.NewRow(ui.NewCol(6, 0, quitLabel),
		ui.NewCol(2, 0, summaryLabel), ui.NewCol(1, 0, summaryPar),
		ui.NewCol(2, 0, failLabel), ui.NewCol(1, 0, failPar)))

	for i := range allprogress {
		allprogress[i].upload, _ = newFileUpload(&ftpServerInfo{
			user:   fUser,
			passwd: fPasswd,
			ipaddr: ips[i]},
			fLocalfile,
			fDestdir)
		bar := ui.NewGauge()
		bar.Percent = 0
		bar.Height = 2
		bar.Label = "{{percent}}%"
		// bar.BorderLabel = ips[i].String()
		bar.BarColor = ui.ColorGreen
		bar.PaddingBottom = 1
		bar.Border = false
		bar.LabelAlign = ui.AlignLeft
		allprogress[i].bar = bar
		label := ui.NewPar(ips[i].String())
		label.Border = false
		// label.Text = ips[i].String()
		label.Height = 2

		// label.Y = 1
		ui.Body.AddRows(ui.NewRow(ui.NewCol(3, 0, label), ui.NewCol(9, 0, bar)))
	}

	for _, item := range allprogress {
		go func(s dashUIBind) {
			var err error
			err = s.upload.upload()
			if err != nil {
				errcount++
			} else {
				sucessCount++
			}
		}(item)
		time.Sleep(time.Millisecond * 10)
	}

	// calculate layout
	ui.Body.Align()

	ui.Render(ui.Body)
	ui.Handle("/timer/1s", func(ui.Event) {
		summaryPar.Text = fmt.Sprintf(" %d/%d", sucessCount, nips)
		failPar.Text = fmt.Sprintf(" %d", errcount)
		for _, item := range allprogress {
			bar := item.bar
			f := item.upload
			if f == nil {
				bar.Label = "OPEN ERROR {{percent}}%"
				bar.PercentColor = ui.ColorRed
				continue
			}

			if f.err != nil {
				bar.Label = f.status + " ERROR {{percent}}%"
				bar.PercentColor = ui.ColorRed
				continue
			}
			if bar.Percent != 100 {
				if f.size == 0 { // empty file,
					if f.status == "DONE" {
						bar.Percent = 100
					}

				} else {
					percent := f.offset * 100 / f.size
					bar.Percent = percent
				}
			}
			bar.Label = f.status + " {{percent}}%"
		}
		ui.Body.Align()
		ui.Clear()
		ui.Render(ui.Body)
	})
	ui.Handle("/sys/kbd/q", func(ui.Event) {
		ui.StopLoop()
	})
	ui.Handle("/sys/wnd/resize", func(e ui.Event) {
		ui.Body.Width = ui.TermWidth()
		ui.Body.Align()
		ui.Clear()
		ui.Render(ui.Body)
	})
	ui.Loop()
	return nil
}
func newFileUpload(server *ftpServerInfo, localfile string, destdir string) (*fileUpload, error) {
	fileInfo, err := os.Stat(localfile)
	if err != nil {
		return nil, err
	}
	fileSize := fileInfo.Size() //获取size
	f, err := os.Open(localfile)
	if err != nil {
		return nil, err
	}
	return &fileUpload{
		fs:      f,
		destdir: destdir,
		server:  server,
		size:    int(fileSize)}, nil
}

func (f *fileUpload) upload() error {

	connstr := fmt.Sprintf("%s:21", f.server.ipaddr.String())

	// because ftp lib has no time, try to connect it just for test
	f.status = "CONNECT"
	try, err := net.DialTimeout("tcp", connstr, time.Second*1)
	if err != nil {
		f.err = err
		return err
	}
	try.Close()
	ftp, err := goftp.Connect(connstr)
	if err != nil {
		f.err = err
		return err
	}
	defer ftp.Quit()
	f.status = "LOGIN"
	if err = ftp.Login(f.server.user, f.server.passwd); err != nil {
		f.err = err
		return err
	}
	f.status = "CHANG DIR"
	if err = ftp.Cwd(f.destdir); err != nil {
		f.err = err
		return err
	}

	f.status = "UPLOAD"

	if err = ftp.Stor(f.fs.Name(), f); err != nil {
		f.err = err
		return err
	}
	f.status = "DONE"
	return nil
}

func (f *fileUpload) Read(b []byte) (int, error) {
	n, err := f.fs.Read(b)
	f.lock.Lock()
	defer f.lock.Unlock()
	f.offset += n
	return n, err
}

func (f *fileUpload) Close() error {
	return f.fs.Close()
}

type simpleUIBind struct {
	bar    *uiprogress.Bar
	upload *fileUpload
}
type dashUIBind struct {
	bar    *ui.Gauge
	upload *fileUpload
}
