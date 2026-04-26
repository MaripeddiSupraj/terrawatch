package main

import "github.com/MaripeddiSupraj/terrawatch/cmd"

// set by goreleaser via ldflags
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cmd.SetVersion(version, commit, date)
	cmd.Execute()
}
