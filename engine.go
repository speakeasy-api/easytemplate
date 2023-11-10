// Package easytemplate provides a templating engine with super powers,
// that allows templates to be written in go text/template syntax,
// with the ability to run javascript snippets in the template, and control
// further templating from within the javascript or other templates.
package easytemplate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"

	"github.com/dop251/goja"
	"github.com/speakeasy-api/easytemplate/internal/template"
	"github.com/speakeasy-api/easytemplate/internal/utils"
	"github.com/speakeasy-api/easytemplate/internal/vm"
)

var (
	// ErrAlreadyRan is returned when the engine has already been ran, and can't be ran again. In order to run the engine again, a new engine must be created.
	ErrAlreadyRan = errors.New("engine has already been ran")
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
	VM *vm.VM
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

// Engine provides the templating engine.
type Engine struct {
	searchLocations []string
	readFS          fs.FS

	templator *template.Templator

	ran     bool
	jsFuncs map[string]func(call CallContext) goja.Value
	jsFiles map[string]string
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

	return e
}

// RunScript runs the provided script file, with the provided data, starting the template engine and templating any templates triggered from the script.
func (e *Engine) RunScript(scriptFile string, data any) error {
	vm, err := e.init(data)
	if err != nil {
		return err
	}

	script, err := e.readFile(scriptFile)
	if err != nil {
		return fmt.Errorf("failed to read script file: %w", err)
	}

	if _, err := vm.Run(scriptFile, string(script)); err != nil {
		return err
	}

	return nil
}

// RunMethod enables calls to global template methods from easytemplate.
func (e *Engine) RunMethod(scriptFile string, data any, fnName string, args ...any) (goja.Value, error) {
	vm, err := e.init(data)
	if err != nil {
		return nil, err
	}

	script, err := e.readFile(scriptFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read script file: %w", err)
	}

	if _, err := vm.Run(scriptFile, string(script)); err != nil {
		return nil, err
	}

	fn, ok := goja.AssertFunction(vm.Get(fnName))
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrFunctionNotFound, fnName)
	}

	gojaArgs := make([]goja.Value, len(args))
	for i, arg := range args {
		gojaArgs[i] = vm.ToValue(arg)
	}
	val, err := fn(goja.Undefined(), gojaArgs...)
	if err != nil {
		return nil, err
	}

	return val, nil
}

// RunTemplate runs the provided template file, with the provided data, starting the template engine and templating the provided template to a file.
func (e *Engine) RunTemplate(templateFile string, outFile string, data any) error {
	vm, err := e.init(data)
	if err != nil {
		return err
	}

	return e.templator.TemplateFile(vm, templateFile, outFile, data)
}

// RunTemplateString runs the provided template file, with the provided data, starting the template engine and templating the provided template, returning the rendered result.
func (e *Engine) RunTemplateString(templateFile string, data any) (string, error) {
	vm, err := e.init(data)
	if err != nil {
		return "", err
	}

	return e.templator.TemplateString(vm, templateFile, data)
}

// RunTemplateStringInput runs the provided input template string, with the provided data, starting the template engine and templating the provided template, returning the rendered result.
func (e *Engine) RunTemplateStringInput(name, template string, data any) (string, error) {
	vm, err := e.init(data)
	if err != nil {
		return "", err
	}

	return e.templator.TemplateStringInput(vm, name, template, data)
}

//nolint:funlen
func (e *Engine) init(data any) (*vm.VM, error) {
	if e.ran {
		return nil, ErrAlreadyRan
	}
	e.ran = true

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
			err := e.templator.TemplateFile(v, templateFile, outFile, data)
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
