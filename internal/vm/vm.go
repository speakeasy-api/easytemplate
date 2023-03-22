// Package vm provides a wrapper around the goja runtime.
package vm

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/console"
	"github.com/dop251/goja_nodejs/require"
	"github.com/go-sourcemap/sourcemap"

	esbuild "github.com/evanw/esbuild/pkg/api"

	"github.com/speakeasy-api/easytemplate/internal/utils"
	"github.com/speakeasy-api/easytemplate/pkg/underscore"
)

var (
	// ErrCompilation is returned when a script fails to compile.
	ErrCompilation = errors.New("script compilation failed")
	// ErrRuntime is returned when a script fails to run.
	ErrRuntime = errors.New("script runtime failure")
)

var lineNumberRegex = regexp.MustCompile(`(?s).*?:([0-9]+):([0-9]+)\([0-9]+\)`)

// VM is a wrapper around the goja runtime.
type VM struct {
	*goja.Runtime
}

type program struct {
	prog      *goja.Program
	sourceMap []byte
}

// New creates a new VM.
func New() (*VM, error) {
	g := goja.New()
	_, err := g.RunString(underscore.JS)
	if err != nil {
		return nil, utils.HandleJSError("failed to init underscore", err)
	}

	new(require.Registry).Enable(g)
	console.Enable(g)

	return &VM{Runtime: g}, nil
}

// Run runs a script in the VM.
func (v *VM) Run(name string, src string) (goja.Value, error) {
	p, err := v.compile(name, src, true)
	if err != nil {
		return nil, err
	}

	res, err := v.Runtime.RunProgram(p.prog)
	if err == nil {
		return res, nil
	}
	var jsErr *goja.Exception
	if !errors.As(err, &jsErr) {
		return nil, fmt.Errorf("failed to run script: %w", errors.Join(ErrRuntime, err))
	}

	m, err := sourcemap.Parse("", p.sourceMap)
	if err != nil {
		return nil, fmt.Errorf("failed to run script: %w", errors.Join(ErrRuntime, err))
	}

	fixedStackTrace, _ := utils.ReplaceAllStringSubmatchFunc(lineNumberRegex, jsErr.String(), func(match []string) (string, error) {
		const expectedMatches = 3

		if len(match) != expectedMatches {
			return match[0], nil
		}

		line, err := strconv.Atoi(match[1])
		if err != nil {
			return match[0], nil //nolint:nilerr
		}
		column, err := strconv.Atoi(match[2])
		if err != nil {
			return match[0], nil //nolint:nilerr
		}

		_, _, line, column, ok := m.Source(line, column)
		if !ok {
			return match[0], nil
		}

		return strings.Replace(match[0], fmt.Sprintf(":%s:%s", match[1], match[2]), fmt.Sprintf(":%d:%d", line, column), 1), nil
	})

	return nil, fmt.Errorf("failed to run script %s: %w", fixedStackTrace, ErrRuntime)
}

// ToObject converts a value to an object.
func (v *VM) ToObject(val goja.Value) *goja.Object {
	return val.ToObject(v.Runtime)
}

func (v *VM) compile(name string, src string, strict bool) (*program, error) {
	// transform src with esbuild -- this ensures we handle typescript
	result := esbuild.Transform(src, esbuild.TransformOptions{
		Target:    esbuild.ES2015,
		Loader:    esbuild.LoaderTS,
		Sourcemap: esbuild.SourceMapExternal,
	})
	if len(result.Errors) > 0 {
		msg := ""
		for _, errMsg := range result.Errors {
			if errMsg.Location == nil {
				msg += fmt.Sprintf("%v @ %v;", errMsg.Text, name)
			} else {
				msg += fmt.Sprintf("%v @ %v %v:%v;", errMsg.Text, name, errMsg.Location.Line, errMsg.Location.Column)
			}
		}
		return nil, fmt.Errorf("%w: %s", ErrCompilation, msg)
	}

	p, err := goja.Compile(name, string(result.Code), strict)
	if err != nil {
		// TODO while its unlikely esbuild will fail to find a compilation error, if it does and goja finds
		// it instead we should look to use the source map to find the error location
		return nil, fmt.Errorf("%w: %s", ErrCompilation, err.Error())
	}

	return &program{
		prog:      p,
		sourceMap: result.Map,
	}, nil
}
