package manifests

import "embed"

// FS embeds the manifests
//
//go:embed virtualcluster/*
var FS embed.FS
