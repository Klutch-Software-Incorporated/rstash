//go:build dev

package ui

import (
	"io/fs"
	"os"
)

var Templates fs.FS
var Static fs.FS
var Site fs.FS
var DevMode = true

func init() {
	Templates = os.DirFS("internal/ui")
	Static = os.DirFS("internal/ui")
	Site = os.DirFS("internal/ui")
}
