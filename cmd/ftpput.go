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
	"fmt"
	"os"
	"sync"

	"github.com/gosuri/uiprogress"
	"github.com/spf13/cobra"

	"net"

	"time"

	"github.com/dutchcoders/goftp"
	"github.com/mythay/anet/util"
)

var fUser, fPasswd, fDestdir, fLocalfile string
var progress = uiprogress.New()

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
		nips := len(ips)
		if _, err := os.Stat(fLocalfile); err != nil {
			return fmt.Errorf("local file '%s' not exist", fLocalfile)
		}

		// try to create all the task
		allprogress := make([]simpleUIBind, nips)
		wg := sync.WaitGroup{}
		wg.Add(nips)
		errmsg := make(chan error, nips)
		for i := range allprogress {
			allprogress[i].upload, err = newFileUpload(&ftpServerInfo{
				user:   fUser,
				passwd: fPasswd,
				ipaddr: ips[i]},
				fLocalfile,
				fDestdir)
			allprogress[i].bar = progress.AddBar(100).AppendCompleted().PrependElapsed()
			allprogress[i].bar.PrependFunc(func(f *fileUpload) uiprogress.DecoratorFunc {
				return func(b *uiprogress.Bar) string {

					if f == nil {
						return fmt.Sprintf("%s :OPEN :ERROR", f.server.ipaddr.String())
					}
					if f.err != nil {
						return fmt.Sprintf("%s :%s :ERROR", f.server.ipaddr.String(), f.status)
					}
					return fmt.Sprintf("%s :%s :     ", f.server.ipaddr.String(), f.status)

				}
			}(allprogress[i].upload))
		}

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
			time.Sleep(time.Millisecond * 100)
		}

		refreshTimer := time.NewTimer(time.Millisecond * 100)
		go func() {
			refreshTimer.Reset(time.Millisecond * 100)
			_, ok := <-refreshTimer.C
			if !ok {
				return
			}
			for _, item := range allprogress {
				f := item.upload
				bar := item.bar
				if bar.Current() == 100 || f.err != nil { // all done, no need to update anymore
					break
				}
				if f.size == 0 { // empty file,
					if f.status == "DONE" {
						bar.Set(99)
						bar.Incr()
					}

				} else {
					percent := f.offset * 100 / f.size
					bar.Set(percent - 1)
					bar.Incr()
				}
			}
		}()

		wg.Wait()
		refreshTimer.Stop()
		progress.Stop()
		errcount := 0

		close(errmsg)
		for msg := range errmsg {
			fmt.Println(msg)
			errcount++
		}

		fmt.Printf("\nSTATISTIC: %d/%d success\n", nips-errcount, nips)
		return nil
	},
}

func init() {
	RootCmd.AddCommand(ftpputCmd)
	ftpputCmd.Flags().StringVarP(&fUser, "username", "u", "pcfactory", "A valid ftp user name")
	ftpputCmd.Flags().StringVarP(&fPasswd, "password", "p", "pcfactory", "Correspoding ftp password")
	ftpputCmd.Flags().StringVarP(&fDestdir, "directory", "d", "/fw", "Ftp server directory to store the file")
	ftpputCmd.Flags().StringVarP(&fLocalfile, "localfile", "l", "App2.out", "local file path to be uploaded")
	progress.Width = 40

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
