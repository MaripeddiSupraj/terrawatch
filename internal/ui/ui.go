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
	bold   = color.New(color.Bold)
	dim    = color.New(color.Faint)

	checkMark = green.Sprint("✓")
	warnMark  = yellow.Sprint("⚠")
	crossMark = red.Sprint("✗")
	arrow     = cyan.Sprint("→")
)

type UI struct {
	out     io.Writer
	isTTY   bool
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

func (u *UI) Header(version string) {
	fmt.Fprintln(u.out)
	cyan.Fprintf(u.out, "  terrawatch %s\n", version)
	fmt.Fprintln(u.out)
}

func (u *UI) ScanStart(total int) {
	fmt.Fprintf(u.out, "  Scanning %d workspace(s)\n\n", total)
}

func (u *UI) WorkspaceScanning(name string) func() {
	if u.isTTY && u.spinner != nil {
		u.spinner.Suffix = fmt.Sprintf("  %s", dim.Sprintf("%-20s scanning...", name))
		u.spinner.Start()
		return func() { u.spinner.Stop() }
	}
	dim.Fprintf(u.out, "  %-20s scanning...\n", name)
	return func() {}
}

func (u *UI) WorkspaceClean(name string) {
	fmt.Fprintf(u.out, "  %s  %-20s %s\n", checkMark, bold.Sprint(name), dim.Sprint("no drift"))
}

func (u *UI) WorkspaceDrift(name string, s terraform.Summary) {
	summary := yellow.Sprintf("+%d ~%d -%d", s.Add, s.Change, s.Destroy)
	fmt.Fprintf(u.out, "  %s  %-20s %s  %s\n",
		warnMark,
		bold.Sprint(name),
		yellow.Sprint("drift detected"),
		summary,
	)
}

func (u *UI) WorkspaceError(name string, err error) {
	fmt.Fprintf(u.out, "  %s  %-20s %s\n",
		crossMark,
		bold.Sprint(name),
		red.Sprintf("error: %v", err),
	)
}

func (u *UI) Divider() {
	dim.Fprintln(u.out, "\n  "+repeat("─", 50))
}

func (u *UI) Summary(scanned, drifted, clean, errs int) {
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
	fmt.Fprintln(u.out)
	fmt.Fprintf(u.out, "  Opening pull requests...\n\n")
}

func (u *UI) PROpened(name, url string) {
	fmt.Fprintf(u.out, "  %s  %-20s %s  %s\n",
		checkMark,
		bold.Sprint(name),
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

func (u *UI) NoDrift() {
	fmt.Fprintln(u.out)
	fmt.Fprintf(u.out, "  %s  %s\n\n", checkMark, green.Sprint("All workspaces are in sync."))
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
