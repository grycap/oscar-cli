package storage

import (
	"io"
	"os"
	"path/filepath"

	"github.com/schollz/progressbar/v3"
	"golang.org/x/term"
)

// TransferOption exposes optional knobs for file transfers.
type TransferOption struct {
	ShowProgress bool
}

func resolveShowProgress(opt *TransferOption) bool {
	if opt == nil {
		return true
	}
	return opt.ShowProgress
}

// transferOptions groups optional settings for file transfers.
type transferOptions struct {
	ShowProgress bool
	Description  string
	TotalBytes   int64
}

// newTransferOptions builds the final options taking defaults into account.
func newTransferOptions(defaultDesc string, total int64, show bool) *transferOptions {
	return &transferOptions{
		ShowProgress: show,
		Description:  defaultDesc,
		TotalBytes:   total,
	}
}

// progressEnabled returns true when a progress bar should be displayed.
func (o *transferOptions) progressEnabled() bool {
	if o == nil {
		return false
	}
	if !o.ShowProgress || o.TotalBytes <= 0 {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// progressReadSeeker wraps an io.ReadSeeker and notifies the bar on read operations.
type progressReadSeeker struct {
	io.ReadSeeker
	bar *progressbar.ProgressBar
}

func newProgressReadSeeker(rs io.ReadSeeker, bar *progressbar.ProgressBar) *progressReadSeeker {
	return &progressReadSeeker{
		ReadSeeker: rs,
		bar:        bar,
	}
}

func (p *progressReadSeeker) Read(buf []byte) (int, error) {
	n, err := p.ReadSeeker.Read(buf)
	if n > 0 && p.bar != nil {
		_ = p.bar.Add(n)
	}
	return n, err
}

// progressWriterAt wraps an io.WriterAt reporting written bytes to the bar.
type progressWriterAt struct {
	io.WriterAt
	bar *progressbar.ProgressBar
}

func newProgressWriterAt(w io.WriterAt, bar *progressbar.ProgressBar) *progressWriterAt {
	return &progressWriterAt{
		WriterAt: w,
		bar:      bar,
	}
}

func (p *progressWriterAt) WriteAt(buf []byte, off int64) (int, error) {
	n, err := p.WriterAt.WriteAt(buf, off)
	if n > 0 && p.bar != nil {
		_ = p.bar.Add(n)
	}
	return n, err
}

// buildProgressBar creates a configured progress bar or returns nil when disabled.
func buildProgressBar(opts *transferOptions) *progressbar.ProgressBar {
	if opts == nil || !opts.progressEnabled() {
		return nil
	}

	bar := progressbar.NewOptions64(
		opts.TotalBytes,
		progressbar.OptionSetDescription(opts.Description),
		progressbar.OptionSetWriter(os.Stdout),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetWidth(35),
		progressbar.OptionThrottle(100),
		progressbar.OptionClearOnFinish(),
		progressbar.OptionSetPredictTime(false),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "=",
			SaucerHead:    ">",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
	)

	return bar
}

func uploadDescription(localPath string) string {
	return "Uploading " + filepath.Base(localPath)
}

func downloadDescription(remotePath string) string {
	return "Downloading " + filepath.Base(remotePath)
}

func finishProgressBar(bar *progressbar.ProgressBar) {
	if bar != nil {
		_ = bar.Finish()
	}
}
