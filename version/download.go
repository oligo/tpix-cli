package version

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"archive/tar"
	"archive/zip"
)

// DownloadCounter counts the number of bytes written to it. It implements to the io.Writer interface
// and we can pass this into io.TeeReader() which will report progress on each write cycle.
type DownloadProgress struct {
	finished   atomic.Uint64
	total      uint64
	reportChan chan float32
	Err        error
}

func (dp *DownloadProgress) Write(p []byte) (int, error) {
	n := len(p)
	dp.finished.Add(uint64(n))

	// compute progress
	progress := float32(dp.finished.Load()) / float32(dp.total)
	dp.reportChan <- progress
	return n, nil
}

func (dp *DownloadProgress) Progress() chan float32 {
	return dp.reportChan
}

func (dp *DownloadProgress) Done() {
	close(dp.reportChan)
}

// Downloader check and download the latest version of TPIX CLI.
type Downloader struct {
	asset   Asset
	destDir string
	client  *http.Client
}

func newDownloadProgress(total uint64) *DownloadProgress {
	return &DownloadProgress{
		total:      total,
		reportChan: make(chan float32, 5),
	}
}

func newDownloader(asset Asset, destDir string) *Downloader {
	if asset.DownloadURL == "" {
		return nil
	}

	c := &http.Client{
		Timeout: 10 * time.Minute,
	}

	return &Downloader{
		client:  c,
		asset:   asset,
		destDir: destDir,
	}

}

func (d *Downloader) get(url string) (*http.Response, error) {
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	return d.client.Do(request)
}

// Download downloads the release file in async manner, and reports its progress.
func (d *Downloader) Download(onFinished func()) *DownloadProgress {
	progress := newDownloadProgress(uint64(d.asset.Size))

	go func() {
		defer progress.Done()
		// download the asset
		resp, err := d.get(d.asset.DownloadURL)
		if err != nil {
			progress.Err = err
			return
		}

		var targetFile *os.File
		targetFile, err = os.OpenFile(filepath.Join(d.destDir, d.asset.Name), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
		if err != nil {
			progress.Err = err
			return
		}

		defer targetFile.Close()

		if n, err := io.Copy(targetFile, io.TeeReader(resp.Body, progress)); err != nil || n != int64(d.asset.Size) {
			progress.Err = errors.New("Download error")
			return
		}

		//uncompress, do not return progress until it finishes.
		err = d.uncompressToDir(targetFile, d.destDir)
		if err != nil {
			progress.Err = err
			return
		}

		if onFinished != nil {
			onFinished()
		}
	}()

	return progress
}

func (d *Downloader) uncompressToDir(targetFile *os.File, destDir string) error {
	isZip := strings.HasSuffix(targetFile.Name(), ".zip")
	isTarball := strings.HasSuffix(targetFile.Name(), ".tar.gz")
	targetFile.Seek(0, io.SeekStart)

	if isTarball {
		err := d.uncompressTarFile(targetFile, destDir)
		if err != nil {
			return err
		}
	} else if isZip {
		err := d.unzipFile(targetFile, destDir)
		if err != nil {
			return err
		}
	} else {
		return errors.New("Unknown release format: " + targetFile.Name())
	}

	return nil
}

func (d *Downloader) uncompressTarFile(targetFile *os.File, destDir string) error {
	// First decompress with gzip
	gz, err := gzip.NewReader(targetFile)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gz.Close()

	// Create a tar Reader from the decompressed stream
	tr := tar.NewReader(gz)
	// Iterate through the files in the archive.
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			// create a directory
			err = os.MkdirAll(filepath.Join(destDir, header.Name), 0755)
			if err != nil {
				return err
			}
		case tar.TypeReg:
			// write a file
			w, err := os.Create(filepath.Join(destDir, header.Name))
			if err != nil {
				return err
			}
			_, err = io.Copy(w, tr)
			if err != nil {
				return err
			}
			w.Close()
		}
	}

	return nil
}

func (d *Downloader) unzipFile(targetFile *os.File, destDir string) error {
	stat, err := targetFile.Stat()
	if err != nil {
		return err
	}
	var r *zip.Reader
	r, err = zip.NewReader(targetFile, stat.Size())
	if err != nil {
		return err
	}

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			// create a directory
			err = os.MkdirAll(filepath.Join(destDir, f.Name), 0755)
			if err != nil {
				return err
			}
			continue
		}

		// normal file, write to destDir directly.
		dest, err := os.Create(filepath.Join(destDir, f.Name))
		if err != nil {
			return err
		}
		defer dest.Close()

		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		_, err = io.Copy(dest, rc)
		if err != nil {
			return err
		}
	}

	return nil

}
