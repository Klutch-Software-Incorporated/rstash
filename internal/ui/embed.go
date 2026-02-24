package ui

import "embed"

//go:embed templates/* templates/admin/*
var Templates embed.FS

//go:embed static/*
var Static embed.FS
