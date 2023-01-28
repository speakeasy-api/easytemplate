// Package underscore provides the underscore-min.js file. Downloaded from https://underscorejs.org/underscore-min.js
package underscore

import (
	_ "embed"
)

//nolint:revive
//go:embed underscore-min.js
var JS string
