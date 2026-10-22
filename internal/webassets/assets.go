package webassets

import "embed"

// Static embeds the production build of the local React/TypeScript dashboard.
//
//go:embed static/*
var Static embed.FS
