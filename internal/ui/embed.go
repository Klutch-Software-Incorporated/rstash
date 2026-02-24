package ui

import "embed"

//go:embed templates/* templates/partials/*
var Templates embed.FS

//go:embed static/*
var Static embed.FS
