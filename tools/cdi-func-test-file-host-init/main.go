//Licensed under the Apache License, Version 2.0 (the "License");
//you may not use this file except in compliance with the License.
//You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
//Unless required by applicable law or agreed to in writing, software
//distributed under the License is distributed on an "AS IS" BASIS,
//WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//See the License for the specific language governing permissions and
//limitations under the License.

package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/klog/v2"

	"kubevirt.io/containerized-data-importer/pkg/util"
	"kubevirt.io/containerized-data-importer/tests/utils"
)

const (
	serviceName   = "cdi-file-host"
	configMapName = serviceName + "-certs"
	certFile      = "tls.crt"
	keyFile       = "tls.key"
)

func main() {
	certDir := flag.String("certDir", "", "")
	inFile := flag.String("inFile", "", "")
	outDir := flag.String("outDir", "", "")
	klog.InitFlags(nil)
	flag.Parse()

	klog.Info("Generating test files")
	ft := &formatTable{
		[]string{""},
		[]string{".gz"},
		[]string{".xz"},
		[]string{".qcow2"},
		[]string{".vmdk"},
		[]string{".vhd"},
		[]string{".vhdx"},
		[]string{".qcow2", ".gz"},
		[]string{".qcow2", ".xz"},
	}

	if err := utils.CreateCertForTestService(util.GetNamespace(), serviceName, configMapName, *certDir, certFile, keyFile); err != nil {
		klog.Fatal(errors.Wrapf(err, "populate certificate directory %s' errored: ", *certDir))
	}

	if err := os.MkdirAll(*outDir, 0777); err != nil {
		klog.Fatal(errors.Wrapf(err, "'mkdir %s' errored: ", *outDir))
	}
	if err := ft.initializeTestFiles(*inFile, *outDir); err != nil {
		klog.Fatal(err)
	}

	if err := util.CopyDir("/tmp/source/", *outDir); err != nil {
		klog.Fatal(err)
	}

	klog.Info("File initialization completed without error.")
}

type formatTable [][]string

func (ft formatTable) initializeTestFiles(inFile, outDir string) error {
	sem := make(chan bool, 2)
	errChan := make(chan error, len(ft))

	reportError := func(err error, msg string, format ...interface{}) {
		e := errors.Wrapf(err, msg, format...)
		klog.Error(e)
		errChan <- e
		return
	}

	for _, fList := range ft {
		sem <- true

		go func(i, o string, f []string) {
			defer func() { <-sem }()
			klog.Infof("Generating file %s\n", f)

			ext := strings.Join(f, "")
			tmpDir := filepath.Join(o, "tmp"+ext)
			if err := os.Mkdir(tmpDir, 0777); err != nil {
				reportError(err, "Error creating temp dir %s", tmpDir)
				return
			}

			defer func() {
				if err := os.RemoveAll(tmpDir); err != nil {
					reportError(err, "Error deleting tmp dir %s", tmpDir)
				}
			}()

			klog.Infof("Mkdir %s\n", tmpDir)

			p, err := utils.FormatTestData(i, tmpDir, f...)
			if err != nil {
				reportError(err, "Error formatting files")
				return
			}

			if err = os.Rename(p, filepath.Join(o, filepath.Base(p))); err != nil {
				reportError(err, "Error moving file %s to %s", p, o)
				return
			}

			klog.Infof("Generated file %q\n", p)
		}(inFile, outDir, fList)
	}
	for i := 0; i < cap(sem); i++ {
		sem <- true
	}
	close(errChan)

	if len(errChan) > 0 {
		for err := range errChan {
			klog.Error(err)
		}
		return errors.New("Error(s) occurred during file conversion")
	}
	return nil
}
