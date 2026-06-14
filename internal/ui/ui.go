package ui

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"golang.org/x/term"

	"github.com/MaripeddiSupraj/terrawatch/pkg/terraform"
)

var (
	green  = color.New(color.FgGreen, color.Bold)
	yellow = color.New(color.FgYellow, color.Bold)
	red    = color.New(color.FgRed, color.Bold)
	cyan   = color.New(color.FgCyan, color.Bold)
	blue   = color.New(color.FgBlue, color.Bold)
	bold   = color.New(color.Bold)
	dim    = color.New(color.Faint)

	checkMark = green.Sprint("✓")
	warnMark  = yellow.Sprint("⚠")
	infoMark  = blue.Sprint("ℹ")
	crossMark = red.Sprint("✗")
	arrow     = cyan.Sprint("→")
)

type UI struct {
	out     io.Writer
	isTTY   bool
	quiet   bool
	spinner *spinner.Spinner
}

func New() *UI {
	isTTY := term.IsTerminal(int(os.Stdout.Fd()))
	u := &UI{
		out:   os.Stdout,
		isTTY: isTTY,
	}
	if isTTY {
		s := spinner.New(spinner.CharSets[14], 80*time.Millisecond)
		s.Writer = os.Stderr
		u.spinner = s
	}
	return u
}

// SetWriter redirects human output (used to keep stdout pure in JSON mode).
func (u *UI) SetWriter(w io.Writer) {
	u.out = w
}

// SetQuiet suppresses informational output. Errors are still printed.
func (u *UI) SetQuiet(quiet bool) {
	u.quiet = quiet
	if quiet {
		u.spinner = nil
	}
}

func (u *UI) Header(version string) {
	if u.quiet {
		return
	}
	fmt.Fprintln(u.out)
	cyan.Fprintf(u.out, "  terrawatch %s\n", version)
	fmt.Fprintln(u.out)
}

func (u *UI) LocalMode(recursive bool) {
	if u.quiet {
		return
	}
	msg := "no config file — local mode (dry-run)"
	if recursive {
		msg = "no config file — local mode, recursive scan (dry-run)"
	}
	dim.Fprintf(u.out, "  %s\n\n", msg)
}

func (u *UI) Binary(binPath string, isOpenTofu bool) {
	if u.quiet {
		return
	}
	label := binPath
	if isOpenTofu {
		label += " (OpenTofu)"
	}
	dim.Fprintf(u.out, "  engine: %s\n\n", label)
}

func (u *UI) ScanStart(total int) {
	if u.quiet {
		return
	}
	fmt.Fprintf(u.out, "  Scanning %d stack(s)\n\n", total)
}

func (u *UI) StackScanning(name string) func() {
	if u.quiet {
		return func() {}
	}
	if u.isTTY && u.spinner != nil {
		u.spinner.Suffix = fmt.Sprintf("  %s", dim.Sprintf("%-20s scanning...", name))
		u.spinner.Start()
		return func() { u.spinner.Stop() }
	}
	dim.Fprintf(u.out, "  %-20s scanning...\n", name)
	return func() {}
}

func (u *UI) StackClean(name string) {
	if u.quiet {
		return
	}
	fmt.Fprintf(u.out, "  %s  %-20s %s\n", checkMark, bold.Sprint(name), dim.Sprint("no drift"))
}

// DriftKind selects the wording shown for a drifted stack. It mirrors the
// detector's classification but lives in the ui package to avoid an import.
type DriftKind int

const (
	DriftGeneric   DriftKind = iota // classification off
	DriftInfra                      // real infrastructure drift
	DriftUnapplied                  // unapplied code changes (reported as drift in all-mode)
)

func (u *UI) StackDrift(name string, s terraform.Summary, hidden int, kind DriftKind) {
	if u.quiet {
		return
	}
	label := "drift detected"
	switch kind {
	case DriftInfra:
		label = "infra drift"
	case DriftUnapplied:
		label = "drift (unapplied code)"
	}
	summary := yellow.Sprintf("+%d ~%d -%d", s.Add, s.Change, s.Destroy)
	extra := ""
	if hidden > 0 {
		extra = dim.Sprintf(" (%d changes hidden by ignore rules)", hidden)
	}
	fmt.Fprintf(u.out, "  %s  %-20s %s  %s%s\n",
		warnMark,
		bold.Sprint(name),
		yellow.Sprint(label),
		summary,
		extra,
	)
}

// StackUnapplied reports plan changes that come from merged-but-unapplied
// code rather than real infrastructure drift (strict mode only).
func (u *UI) StackUnapplied(name string, s terraform.Summary) {
	if u.quiet {
		return
	}
	summary := dim.Sprintf("+%d ~%d -%d", s.Add, s.Change, s.Destroy)
	fmt.Fprintf(u.out, "  %s  %-20s %s  %s\n",
		infoMark,
		bold.Sprint(name),
		blue.Sprint("unapplied changes"),
		summary,
	)
}

func (u *UI) StackError(name string, err error) {
	fmt.Fprintf(u.out, "  %s  %-20s %s\n",
		crossMark,
		bold.Sprint(name),
		red.Sprintf("error: %v", err),
	)
}

func (u *UI) Divider() {
	if u.quiet {
		return
	}
	dim.Fprintln(u.out, "\n  "+repeat("─", 50))
}

func (u *UI) Summary(scanned, drifted, clean, errs int) {
	if u.quiet {
		return
	}
	fmt.Fprintf(u.out, "  %s scanned  %s  %s drifted  %s  %s clean",
		bold.Sprint(scanned),
		dim.Sprint("·"),
		yellow.Sprint(drifted),
		dim.Sprint("·"),
		green.Sprint(clean),
	)
	if errs > 0 {
		fmt.Fprintf(u.out, "  %s  %s", dim.Sprint("·"), red.Sprint(errs)+" errors")
	}
	fmt.Fprintln(u.out)
}

func (u *UI) PRStart() {
	if u.quiet {
		return
	}
	fmt.Fprintln(u.out)
	fmt.Fprintf(u.out, "  Opening pull requests...\n\n")
}

func (u *UI) PROpened(name, url string, existing bool) {
	if u.quiet {
		return
	}
	label := dim.Sprint("PR opened")
	if existing {
		label = dim.Sprint("PR exists")
	}
	fmt.Fprintf(u.out, "  %s  %-20s %s  %s  %s\n",
		checkMark,
		bold.Sprint(name),
		label,
		arrow,
		color.New(color.FgCyan, color.Underline).Sprint(url),
	)
}

// PRClosed reports a drift PR that was auto-closed because the stack is clean.
func (u *UI) PRClosed(name, url string) {
	if u.quiet {
		return
	}
	fmt.Fprintf(u.out, "  %s  %-20s %s  %s  %s\n",
		checkMark,
		bold.Sprint(name),
		dim.Sprint("drift resolved — PR closed"),
		arrow,
		color.New(color.FgCyan, color.Underline).Sprint(url),
	)
}

func (u *UI) PRError(name string, err error) {
	fmt.Fprintf(u.out, "  %s  %-20s %s\n",
		crossMark,
		bold.Sprint(name),
		red.Sprintf("pr failed: %v", err),
	)
}

// Warn prints a non-fatal warning that never affects the exit code.
func (u *UI) Warn(name string, err error) {
	fmt.Fprintf(u.out, "  %s  %-20s %s\n",
		warnMark,
		bold.Sprint(name),
		yellow.Sprintf("warning: %v", err),
	)
}

func (u *UI) NoDrift() {
	if u.quiet {
		return
	}
	fmt.Fprintln(u.out)
	fmt.Fprintf(u.out, "  %s  %s\n\n", checkMark, green.Sprint("All stacks are in sync."))
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
