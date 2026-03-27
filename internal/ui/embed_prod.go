//go:build !dev

package ui

import (
	"embed"
	"io/fs"
)

//go:embed templates/* templates/partials/*
var embeddedTemplates embed.FS

//go:embed static/*
var embeddedStatic embed.FS

//go:embed all:site
var embeddedSite embed.FS

var Templates fs.FS
var Static fs.FS
var Site fs.FS
var DevMode = false

func init() {
	Templates = embeddedTemplates
	Static = embeddedStatic
	Site = embeddedSite
}
