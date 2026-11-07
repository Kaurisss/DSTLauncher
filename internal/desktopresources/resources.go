//go:build desktop && windows

// Package desktopresources embeds the Windows desktop icon and DPI manifest.
package desktopresources

//go:generate go run github.com/tc-hib/go-winres@v0.3.3 make --in winres.json --arch amd64,386,arm64 --out rsrc
