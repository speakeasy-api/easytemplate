// Package easytemplate provides a templating engine with super powers,
// that allows templates to be written in go text/template syntax,
// with the ability to run javascript snippets in the template, and control
// further templating from within the javascript or other templates.
package easytemplate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"

	"github.com/dop251/goja"
	"github.com/speakeasy-api/easytemplate/internal/template"
	"github.com/speakeasy-api/easytemplate/internal/utils"
	"github.com/speakeasy-api/easytemplate/internal/vm"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

var (
	// ErrAlreadyInitialized is returned when the engine has already been initialized.
	ErrAlreadyInitialized = errors.New("engine has already been initialized")
	// ErrNotInitialized is returned when the engine has not been initialized.
	ErrNotInitialized = errors.New("engine has not been initialized")
	// ErrReserved is returned when a template or js function is reserved and can't be overridden.
	ErrReserved = errors.New("function is a reserved function and can't be overridden")
	// ErrInvalidArg is returned when an invalid argument is passed to a function.
	ErrInvalidArg = errors.New("invalid argument")
	// ErrTemplateCompilation is returned when a template fails to compile.
	ErrTemplateCompilation = errors.New("template compilation failed")
	// ErrFunctionNotFound Function does not exist in script.
	ErrFunctionNotFound = errors.New("failed to find function")
)

// CallContext is the context that is passed to go functions when called from js.
type CallContext struct {
	goja.FunctionCall
	VM  *vm.VM
	Ctx context.Context //nolint:containedctx // runtime context is necessarily stored in a struct as it jumps from Go to JS.
}

// Opt is a function that configures the Engine.
type Opt func(*Engine)

// WithSearchLocations allows for providing additional locations to search for templates and scripts.
func WithSearchLocations(searchLocations []string) Opt {
	return func(e *Engine) {
		e.searchLocations = searchLocations
	}
}

// WithReadFileSystem sets the file system to use for reading files. This is useful for embedded files or reading from locations other than disk.
func WithReadFileSystem(fs fs.FS) Opt {
	return func(e *Engine) {
		e.readFS = fs
	}
}

// WithWriteFunc sets the write function to use for writing files. This is useful for writing to locations other than disk.
func WithWriteFunc(writeFunc func(string, []byte) error) Opt {
	return func(e *Engine) {
		e.templator.WriteFunc = writeFunc
	}
}

// WithTemplateFuncs allows for providing additional template functions to the engine, available to all templates.
func WithTemplateFuncs(funcs map[string]any) Opt {
	return func(e *Engine) {
		for k, v := range funcs {
			if _, ok := e.templator.TmplFuncs[k]; ok {
				panic(fmt.Errorf("%s is reserved: %w", k, ErrReserved))
			}

			e.templator.TmplFuncs[k] = v
		}
	}
}

// WithJSFuncs allows for providing additional functions available to javascript in the engine.
func WithJSFuncs(funcs map[string]func(call CallContext) goja.Value) Opt {
	return func(e *Engine) {
		for k, v := range funcs {
			if _, ok := e.jsFuncs[k]; ok {
				panic(fmt.Errorf("%s is reserved: %w", k, ErrReserved))
			}

			e.jsFuncs[k] = v
		}
	}
}

// WithJSFiles allows for providing additional javascript files to be loaded into the engine.
func WithJSFiles(files map[string]string) Opt {
	return func(e *Engine) {
		e.jsFiles = files
	}
}

// WithTracer attaches an OpenTelemetry tracer to the engine and enables tracing support.
func WithTracer(t trace.Tracer) Opt {
	return func(e *Engine) {
		e.tracer = t
	}
}

// WithDebug enables debug mode for the engine, which will log additional information when errors occur.
func WithDebug() Opt {
	return func(e *Engine) {
		e.templator.Debug = true
	}
}

// Engine provides the templating engine.
type Engine struct {
	searchLocations []string
	readFS          fs.FS

	templator *template.Templator

	jsFuncs map[string]func(call CallContext) goja.Value
	jsFiles map[string]string

	tracer trace.Tracer

	vm *vm.VM
}

// New creates a new Engine with the provided options.
func New(opts ...Opt) *Engine {
	t := &template.Templator{
		// Reserving the names for now
		TmplFuncs: map[string]any{
			"templateFile":        nil,
			"templateString":      nil,
			"templateStringInput": nil,
		},
		WriteFunc: func(s string, b []byte) error {
			return os.WriteFile(s, b, os.ModePerm)
		},
	}

	e := &Engine{
		templator: t,
		jsFuncs:   map[string]func(call CallContext) goja.Value{},
		jsFiles:   map[string]string{},
	}

	t.ReadFunc = e.readFile

	e.jsFuncs = map[string]func(call CallContext) goja.Value{
		"require":                e.require,
		"recurse":                e.recurseJS,
		"templateFile":           e.templateFileJS,
		"templateString":         e.templateStringJS,
		"templateStringInput":    e.templateStringInputJS,
		"registerTemplateFunc":   e.registerTemplateFunc,
		"unregisterTemplateFunc": e.unregisterTemplateFunc,
	}

	for _, opt := range opts {
		opt(e)
	}

	if e.tracer == nil {
		e.tracer = noop.NewTracerProvider().Tracer("easytemplate")
	}

	return e
}

// Init initializes the engine with global data available to all following methods, and should be called before any other methods are called but only once.
// When using any of the Run or Template methods after init, they will share the global data, but just be careful they will also share any changes made to the environment
// by previous runs.
func (e *Engine) Init(ctx context.Context, data any) error {
	v, err := e.init(ctx, data)
	if err != nil {
		return err
	}

	e.vm = v

	return nil
}

// RunScript runs the provided script file within the environment initialized by Init.
// This is useful for setting up the environment with global variables and functions,
// or running code that is not directly related to templating but might setup the environment for templating.
func (e *Engine) RunScript(scriptFile string) error {
	if e.vm == nil {
		return ErrNotInitialized
	}

	script, err := e.readFile(scriptFile)
	if err != nil {
		return fmt.Errorf("failed to read script file: %w", err)
	}

	if _, err := e.vm.Run(scriptFile, string(script)); err != nil {
		return err
	}

	return nil
}

// RunFunction will run the named function if it already exists within the environment, for example if it was defined in a script run by RunScript.
// The provided args will be passed to the function, and the result will be returned.
func (e *Engine) RunFunction(fnName string, args ...any) (goja.Value, error) {
	if e.vm == nil {
		return nil, ErrNotInitialized
	}

	fn, ok := goja.AssertFunction(e.vm.Get(fnName))
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrFunctionNotFound, fnName)
	}

	gojaArgs := make([]goja.Value, len(args))
	for i, arg := range args {
		gojaArgs[i] = e.vm.ToValue(arg)
	}
	val, err := fn(goja.Undefined(), gojaArgs...)
	if err != nil {
		return nil, err
	}

	return val, nil
}

// TemplateFile runs the provided template file, with the provided data and writes the result to the provided outFile.
func (e *Engine) TemplateFile(templateFile string, outFile string, data any) error {
	if e.vm == nil {
		return ErrNotInitialized
	}

	return e.templator.TemplateFile(e.vm, templateFile, outFile, data)
}

// TemplateString runs the provided template file, with the provided data and returns the rendered result.
func (e *Engine) TemplateString(templateFilePath string, data any) (string, error) {
	if e.vm == nil {
		return "", ErrNotInitialized
	}

	return e.templator.TemplateString(e.vm, templateFilePath, data)
}

// TemplateStringInput runs the provided template string, with the provided data and returns the rendered result.
func (e *Engine) TemplateStringInput(name, template string, data any) (string, error) {
	if e.vm == nil {
		return "", ErrNotInitialized
	}

	return e.templator.TemplateStringInput(e.vm, name, template, data)
}

//nolint:funlen
func (e *Engine) init(ctx context.Context, data any) (*vm.VM, error) {
	if e.vm != nil {
		return nil, ErrAlreadyInitialized
	}

	v, err := vm.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create vm: %w", err)
	}

	for name, content := range e.jsFiles {
		_, err := v.RunString(content)
		if err != nil {
			return nil, fmt.Errorf("failed to init %s: %w", name, err)
		}
	}

	for name, fn := range e.jsFuncs {
		wrappedFn := func(fn func(call CallContext) goja.Value) func(call goja.FunctionCall) goja.Value {
			return func(call goja.FunctionCall) goja.Value {
				return fn(CallContext{
					FunctionCall: call,
					VM:           v,
					Ctx:          ctx,
				})
			}
		}(fn)

		if err := v.Set(name, wrappedFn); err != nil {
			return nil, fmt.Errorf("failed to set js function %s: %w", name, err)
		}
	}

	// This need to have the vm passed in so that the functions can be called
	e.templator.TmplFuncs["templateFile"] = func(v *vm.VM) func(string, string, any) (string, error) {
		return func(templateFile, outFile string, data any) (string, error) {
			var err error
			_, span := e.tracer.Start(ctx, "templateFile", trace.WithAttributes(
				attribute.String("templateFile", templateFile),
				attribute.String("outFile", outFile),
			))
			defer func() {
				span.RecordError(err)
				if err != nil {
					span.SetStatus(codes.Error, err.Error())
				}
				span.End()
			}()

			err = e.templator.TemplateFile(v, templateFile, outFile, data)
			if err != nil {
				return "", err
			}

			return "", nil
		}
	}(v)
	e.templator.TmplFuncs["templateString"] = func(v *vm.VM) func(string, any) (string, error) {
		return func(templateFile string, data any) (string, error) {
			templated, err := e.templator.TemplateString(v, templateFile, data)
			if err != nil {
				return "", err
			}

			return templated, nil
		}
	}(v)
	e.templator.TmplFuncs["templateStringInput"] = func(v *vm.VM) func(string, string, any) (string, error) {
		return func(name, template string, data any) (string, error) {
			templated, err := e.templator.TemplateStringInput(v, name, template, data)
			if err != nil {
				return "", err
			}

			return templated, nil
		}
	}(v)
	e.templator.TmplFuncs["recurse"] = func(v *vm.VM) func(int) (string, error) {
		return func(numTimes int) (string, error) {
			templated, err := e.templator.Recurse(v, numTimes)
			if err != nil {
				return "", err
			}

			return templated, nil
		}
	}(v)

	if _, err := v.Run("initCreateComputedContextObject", `function createComputedContextObject() { return {}; }`); err != nil {
		return nil, utils.HandleJSError("failed to init createComputedContextObject", err)
	}

	globalComputed, err := v.Run("globalCreateComputedContextObject", `createComputedContextObject();`)
	if err != nil {
		return nil, utils.HandleJSError("failed to init globalComputed", err)
	}

	e.templator.SetContextData(data, globalComputed)
	if err := v.Set("context", &template.Context{
		Global:         data,
		GlobalComputed: globalComputed,
		Local:          data,
		LocalComputed:  globalComputed,
	}); err != nil {
		return nil, fmt.Errorf("failed to set context: %w", err)
	}

	return v, nil
}

func (e *Engine) unregisterTemplateFunc(call CallContext) goja.Value {
	name := call.Argument(0).String()
	if _, ok := e.templator.TmplFuncs[name]; !ok {
		panic(call.VM.NewGoError(fmt.Errorf("%w: template function %s does not exist", ErrReserved, name)))
	}

	delete(e.templator.TmplFuncs, name)

	return goja.Undefined()
}

func (e *Engine) require(call CallContext) goja.Value {
	vm := call.VM

	scriptPath := call.Argument(0).String()

	script, err := e.readFile(scriptPath)
	if err != nil {
		currentCallStack := vm.CaptureCallStack(0, nil)
		currentScript := currentCallStack[1].SrcName()
		relativePath := path.Join(path.Dir(currentScript), scriptPath)
		script, err = e.readFile(relativePath)
	}
	if err != nil {
		panic(vm.NewGoError(err))
	}

	if _, err := vm.Run(scriptPath, string(script)); err != nil {
		panic(vm.NewGoError(err))
	}

	return goja.Undefined()
}

func (e *Engine) registerTemplateFunc(call CallContext) goja.Value {
	name := call.Argument(0).String()
	fn, ok := goja.AssertFunction(call.Argument(1))
	if !ok {
		panic(call.VM.NewGoError(fmt.Errorf("%w: second argument must be a function", ErrInvalidArg)))
	}

	if _, ok := e.templator.TmplFuncs[name]; ok {
		panic(call.VM.NewGoError(fmt.Errorf("%w: template function %s already exists", ErrReserved, name)))
	}

	e.templator.TmplFuncs[name] = func(fn goja.Callable) func(args ...interface{}) any {
		return func(args ...interface{}) any {
			callableArgs := make([]goja.Value, len(args))
			for i, arg := range args {
				callableArgs[i] = call.VM.ToValue(arg)
			}

			val, err := fn(goja.Undefined(), callableArgs...)
			if err != nil {
				panic(err)
			}

			return val.Export()
		}
	}(fn)

	return goja.Undefined()
}

func (e *Engine) readFile(file string) ([]byte, error) {
	filePath := file

	for _, dir := range e.searchLocations {
		searchPath := path.Join(dir, filePath)

		if e.readFS != nil {
			if _, err := fs.Stat(e.readFS, searchPath); err == nil {
				filePath = searchPath
				break
			}
		} else {
			if _, err := os.Stat(searchPath); err == nil {
				filePath = searchPath
				break
			}
		}
	}

	if e.readFS != nil {
		return fs.ReadFile(e.readFS, filePath)
	}
	return os.ReadFile(filePath)
}
