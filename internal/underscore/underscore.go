// Package underscore provides the underscore-min.js file.
package underscore

import (
	_ "embed"
)

//nolint:revive
//go:embed underscore-min.js
var JS string
