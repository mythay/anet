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
	"path/filepath"
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
		destfile := filepath.Base(fLocalfile)
		tsk := &batchUpload{
			ftpServerInfo: ftpServerInfo{
				user:    fUser,
				passwd:  fPasswd,
				destdir: fDestdir},
			localfile: fLocalfile,
			destfile:  destfile,
			verbose:   true}

		progress.Start()
		wg := sync.WaitGroup{}
		wg.Add(nips)
		errmsg := make(chan error, nips)

		for _, ip := range ips {
			go func(nip net.IP) {
				var err error
				defer wg.Done()
				err = tsk.upload(nip)
				if err != nil {
					errmsg <- err
				}
			}(ip)
		}

		wg.Wait()
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
	user    string
	passwd  string
	destdir string
}

type batchUpload struct {
	ftpServerInfo
	localfile string
	destfile  string
	verbose   bool
}

type pFile struct {
	*os.File
	offset int
	bar    *uiprogress.Bar
}

func newPFile(fpath string) (*pFile, error) {
	f, err := os.Open(fpath)
	if err != nil {
		return nil, err
	}
	size := fileSize(fpath)
	bar := progress.AddBar(size).AppendCompleted().PrependElapsed()
	return &pFile{f, 0, bar}, nil
}

func (f *pFile) Read(b []byte) (n int, err error) {
	n, err = f.File.Read(b)
	f.offset += n
	f.bar.Set(f.offset - 1)
	f.bar.Incr()
	return n, err
}
func fileSize(path string) int {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return 0
	}
	fileSize := fileInfo.Size() //获取size
	return int(fileSize)
}
func (batch *batchUpload) upload(destip net.IP) error {
	var err error
	iscomplete := false
	connstr := fmt.Sprintf("%s:21", destip.String())

	// because ftp lib has no time, try to connect it just for test
	try, err := net.DialTimeout("tcp", connstr, time.Second*1)
	if err != nil {
		return err
	}
	try.Close()

	f, err := newPFile(batch.localfile)
	if err != nil {
		return err
	}
	defer f.Close()

	f.bar.PrependFunc(func(b *uiprogress.Bar) string {
		if err != nil {
			return fmt.Sprintf("%s :ERROR", destip)
		}
		if iscomplete {
			return fmt.Sprintf("%s :DONE ", destip)
		}
		return fmt.Sprintf("%s :     ", destip)

	})
	defer f.bar.Incr()
	ftp, err := goftp.Connect(fmt.Sprintf("%s:21", destip))
	if err != nil {
		return err
	}
	defer ftp.Quit()
	if err = ftp.Login(batch.user, batch.passwd); err != nil {
		return err
	}
	if err = ftp.Cwd(batch.destdir); err != nil {
		return err
	}

	if err = ftp.Stor(batch.destfile, f); err != nil {
		return err
	}
	iscomplete = true

	return nil

}
