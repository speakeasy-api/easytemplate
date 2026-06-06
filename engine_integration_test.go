package easytemplate_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/dop251/goja"
	"github.com/speakeasy-api/easytemplate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEngine_RunScript_Success(t *testing.T) {
	files, err := os.ReadDir("./testdata/expected")
	require.NoError(t, err)

	expectedFiles := make(map[string]string, len(files))

	for _, file := range files {
		data, err := os.ReadFile("./testdata/expected/" + file.Name())
		require.NoError(t, err)

		expectedFiles[file.Name()] = string(data)
	}

	e := easytemplate.New(
		easytemplate.WithSearchLocations([]string{"./testdata"}),
		easytemplate.WithWriteFunc(func(outFile string, data []byte) error {
			expectedData, ok := expectedFiles[outFile]
			if ok {
				assert.Equal(t, expectedData, string(data))
				delete(expectedFiles, outFile)
			} else {
				require.NoError(t, os.WriteFile("./testdata/expected/"+outFile, data, 0o644))
			}

			return nil
		}),
		easytemplate.WithJSFuncs(map[string]func(call easytemplate.CallContext) goja.Value{
			"multiply": func(call easytemplate.CallContext) goja.Value {
				a := call.Argument(0).ToInteger()
				b := call.Argument(1).ToInteger()

				return call.VM.ToValue(a * b)
			},
		}),
		easytemplate.WithTemplateFuncs(map[string]any{
			"toFloatWithPrecision": func(i int64, precision int) string {
				return fmt.Sprintf("%.*f", precision, float64(i))
			},
		}),
	)

	err = e.Init(context.Background(), map[string]interface{}{
		"Test": "global",
	})
	require.NoError(t, err)

	err = e.RunScript(context.Background(), "scripts/test.js")
	require.NoError(t, err)

	assert.Empty(t, expectedFiles, "not all expected files were written")
}

func TestEngine_GoRuntimePanicCaughtByJSTryCatch(t *testing.T) {
	// Verifies that Go runtime panics (e.g. nil-pointer dereference) from
	// native functions are converted to GoError exceptions that JS try/catch
	// can handle, rather than bypassing JS error handling entirely.
	type myStruct struct {
		Name string
	}

	e := easytemplate.New(
		easytemplate.WithJSFuncs(map[string]func(call easytemplate.CallContext) goja.Value{
			"panicWithNilDeref": func(call easytemplate.CallContext) goja.Value {
				var s *myStruct                // nil pointer
				return call.VM.ToValue(s.Name) // will panic: nil pointer dereference
			},
		}),
		easytemplate.WithJSFiles(map[string]string{
			"init.js": `
				function testCatch() {
					try {
						panicWithNilDeref();
						return "not caught";
					} catch (e) {
						return "caught";
					}
				}
			`,
		}),
	)

	err := e.Init(context.Background(), nil)
	require.NoError(t, err)

	// The JS try/catch should catch the Go panic, and the function should
	// return "caught" instead of crashing the process.
	val, err := e.RunFunction(context.Background(), "testCatch")
	require.NoError(t, err)
	assert.Equal(t, "caught", val.Export())
}

func TestEngine_GoRuntimePanicIncludesStackTrace(t *testing.T) {
	// Verifies that when a Go runtime panic escapes to the caller (no JS
	// try/catch), the returned error contains a PanicError with both Go and
	// JS stack traces.
	type myStruct struct {
		Name string
	}

	e := easytemplate.New(
		easytemplate.WithJSFuncs(map[string]func(call easytemplate.CallContext) goja.Value{
			"panicWithNilDeref": func(call easytemplate.CallContext) goja.Value {
				var s *myStruct                // nil pointer
				return call.VM.ToValue(s.Name) // will panic: nil pointer dereference
			},
		}),
		easytemplate.WithJSFiles(map[string]string{
			"init.js": `
				function outerFunc() {
					return innerFunc();
				}
				function innerFunc() {
					return panicWithNilDeref();
				}
			`,
		}),
	)

	err := e.Init(context.Background(), nil)
	require.NoError(t, err)

	_, err = e.RunFunction(context.Background(), "outerFunc")
	require.Error(t, err)

	// The error should wrap a GoError whose value is a *PanicError.
	var gojaErr *goja.Exception
	require.ErrorAs(t, err, &gojaErr)

	// Extract the GoError value from the exception.
	obj := gojaErr.Value().ToObject(e.Runtime())
	raw := obj.Get("value").Export()

	panicErr, ok := raw.(*easytemplate.PanicError)
	require.True(t, ok, "expected *PanicError, got %T", raw)

	// Go stack should contain the panic origin.
	assert.Contains(t, panicErr.GoStack, "runtime/panic.go")
	assert.Contains(t, panicErr.GoStack, "recoverGoRuntimePanic")

	// JS stack should contain the JS call chain.
	jsNames := make([]string, len(panicErr.JSStack))
	for i, frame := range panicErr.JSStack {
		jsNames[i] = frame.FuncName()
	}
	assert.Contains(t, jsNames, "innerFunc")
	assert.Contains(t, jsNames, "outerFunc")

	// Should still be unwrappable to ErrNativePanic.
	assert.ErrorIs(t, panicErr, easytemplate.ErrNativePanic)
}

func TestEngine_ContextOutFile(t *testing.T) {
	// Verifies that context.OutFile is populated during a templateFile render,
	// restored across nested templateFile calls, and cleared after the
	// outermost render returns. JS helpers can use it to derive paths
	// relative to the file currently being templated.
	var (
		captured []string
		eng      *easytemplate.Engine
	)

	var writes []string
	eng = easytemplate.New(
		easytemplate.WithSearchLocations([]string{"./testdata"}),
		easytemplate.WithWriteFunc(func(outFile string, data []byte) error {
			writes = append(writes, string(data))
			return nil
		}),
		easytemplate.WithTemplateFuncs(map[string]any{
			"captureOutFile": func() string {
				ctx := eng.Runtime().Get("context").ToObject(eng.Runtime())
				captured = append(captured, ctx.Get("OutFile").String())
				return ""
			},
		}),
	)
	e := eng

	require.NoError(t, e.Init(context.Background(), nil))

	// Before any templateFile call, OutFile should be empty.
	rootCtx := e.Runtime().Get("context").ToObject(e.Runtime())
	assert.Empty(t, rootCtx.Get("OutFile").String())

	// templateFile sets OutFile for the duration of the render.
	require.NoError(t, e.TemplateFile(context.Background(),
		"templates/outfile_outer.stmpl", "out/outer.txt", nil))

	// After return: restored to empty.
	assert.Empty(t, rootCtx.Get("OutFile").String())

	// During the outer render, OutFile == "out/outer.txt". The outer template
	// nests a templateFile call that should overwrite to "out/inner.txt"
	// then restore "out/outer.txt".
	assert.Equal(t, []string{
		"out/outer.txt", // top of outer template
		"out/inner.txt", // top of inner template (nested)
		"out/outer.txt", // resumed outer template after nested call
	}, captured)

	// {{.OutFile}} must also resolve inside text/template renders.
	require.Len(t, writes, 2)
	assert.Contains(t, writes[1], "tmpl=out/outer.txt") // outer.txt written last
	assert.Contains(t, writes[0], "tmpl=out/inner.txt") // inner.txt written first
}
