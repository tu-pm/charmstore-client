package iomon

import (
	"fmt"
	"io"
	"strings"
)

// StatusSetter is used to indicate the current progress status.
type StatusSetter interface {
	SetStatus(s Status)
}

// Status indicates the current status of the I/O transfer.
type Status struct {
	// Current holds the current number of transferred bytes.
	Current int64
	// Total holds the total number of bytes in the transfer.
	Total int64

	// TODO add rate, expected time
}

// String provides a textual representation of the status.
func (s Status) String() string {
	percent := 100
	if s.Total != 0 {
		percent = int(float64(s.Current)/float64(s.Total)*100 + 0.5)
	}
	return fmt.Sprintf("%3d%% %9s", percent, FormatByteCount(s.Current))
}

// NewPrinter returns a new Printer instance that
// prints the current status to the given writer,.
// The name holds the name of whatever is being
// transferred.
func NewPrinter(w io.Writer, name string) *Printer {
	return &Printer{
		w:    w,
		name: name,
	}
}

var _ StatusSetter = (*Printer)(nil)

// Printer implements StatusSetter by printing status messages
// to a writer, erasing each one by printing a carriage return
// at the start of each message.
type Printer struct {
	w    io.Writer
	name string
	// prevWidth holds the number of characters printed by
	// the most recent call to SetStatus.
	prevWidth int
}

// SetStatus implements StatusSetter.SetStatus by
// printing the status as text.
func (p *Printer) SetStatus(status Status) {
	s := fmt.Sprintf("\r%-45s %s", p.name, status)
	width := len(s)
	if p.prevWidth > width {
		// For whatever reason, the line we printed before
		// was longer than this one, so print extra spaces
		// to erase it.
		s += strings.Repeat(" ", p.prevWidth-width)
	}
	p.prevWidth = width
	p.w.Write([]byte(s))
}

// Done indicates that the transfer has stopped by printing
// a newline to leave the last printed status message intact.
func (p *Printer) Done() {
	p.w.Write([]byte("\n"))
}

// Clear clears the status by printing spaces over the
// existing text.
func (p *Printer) Clear() {
	p.w.Write([]byte("\r" + strings.Repeat(" ", p.prevWidth) + "\r"))
	p.prevWidth = 0
}

const (
	KiB = 1024
	MiB = 1024 * KiB
	GiB = 1024 * MiB
)

// FormatByteCount returns a string representation of
// the given number formatted so as to be user readable
// in progress reports.
func FormatByteCount(n int64) string {
	switch {
	case n < 10*MiB:
		return fmt.Sprintf("%.0fKiB", float64(n)/KiB)
	case n < 10*GiB:
		return fmt.Sprintf("%.1fMiB", float64(n)/MiB)
	default:
		return fmt.Sprintf("%.1fGiB", float64(n)/GiB)
	}
}
