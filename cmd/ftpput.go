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

// ftpputCmd represents the put command
var ftpputCmd = &cobra.Command{
	Use:   "ftpput [ip]...",
	Short: "upload file to ftp server",
	Long: `upload file to multiple ftp server at the same time.
For example:
	anet ftpput 192.168.1.1 192.168.1.2`,
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

		// return uploadSimpleView(ips)
		return uploadDashView(ips)
	},
}

func init() {
	RootCmd.AddCommand(ftpputCmd)
	ftpputCmd.Flags().StringVarP(&fUser, "username", "u", "pcfactory", "A valid ftp user name")
	ftpputCmd.Flags().StringVarP(&fPasswd, "password", "p", "pcfactory", "Correspoding ftp password")
	ftpputCmd.Flags().StringVarP(&fDestdir, "directory", "d", "/fw", "Ftp server directory to store the file")
	ftpputCmd.Flags().StringVarP(&fLocalfile, "localfile", "l", "App2.out", "local file path to be uploaded")

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
	progress.Width = 40
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

	g := ui.NewGauge()
	g.Percent = 50
	g.Height = 3

	g.BorderLabel = "Gauge"
	g.BarColor = ui.ColorRed
	g.BorderFg = ui.ColorWhite
	g.BorderLabelFg = ui.ColorCyan
	ui.Body.AddRows(
		ui.NewRow(
			ui.NewCol(12, 0, g)))
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
	// calculate layout
	ui.Body.Align()

	ui.Render(ui.Body)

	ui.Render(g) // feel free to call Render, it's async and non-block
	ui.Handle("/sys/kbd/q", func(ui.Event) {
		ui.StopLoop()
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
