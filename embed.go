package minidon

import "embed"

//go:embed web/dist
var StaticFS embed.FS
