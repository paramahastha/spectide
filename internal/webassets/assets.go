package webassets

import "embed"

// Static contains the compiled dashboard asset directory. The development build
// currently ships the same dependency-free local shell so the binary remains offline runnable.
//
//go:embed static/*
var Static embed.FS
