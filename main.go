// gh-ghs: a GitHub account switcher with per-directory guardrails.
// Installed as a gh extension (`gh ghs …`); `ghs link` adds symlinks so
// `git ghs …` and bare `ghs …` work identically.
package main

import (
	"os"

	"github.com/OSSMafia/gh-ghs/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
