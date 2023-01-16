// Package easytemplate provides a templating engine with super powers,
// that allows templates to be written in go text/template syntax,
// with the ability to run javascript snippets in the template, and control
// further templating from within the javascript or other templates.
package easytemplate

import (
	"fmt"
	"io/fs"
	"os"
	"path"

	"github.com/robertkrimen/otto"
	// provides underscore support for js interpreted by the engine.
	_ "github.com/robertkrimen/otto/underscore"
	"github.com/speakeasy-api/easytemplate/internal/template"
)

var (
	// ErrAlreadyRan is returned when the engine has already been ran, and can't be ran again. In order to run the engine again, a new engine must be created.
	ErrAlreadyRan = fmt.Errorf("engine has already been ran")
	// ErrReserved is returned when a template or js function is reserved and can't be overridden.
	ErrReserved = fmt.Errorf("function is a reserved function and can't be overridden")
)

// Opt is a function that configures the Engine.
type Opt func(*Engine)

// WithBaseDir sets the base directory for finding scripts and templates. Allowing for relative paths.
func WithBaseDir(baseDir string) Opt {
	return func(e *Engine) {
		e.baseDir = baseDir
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
func WithJSFuncs(funcs map[string]func(call otto.FunctionCall) otto.Value) Opt {
	return func(e *Engine) {
		for k, v := range funcs {
			if _, ok := e.jsFuncs[k]; ok {
				panic(fmt.Errorf("%s is reserved: %w", k, ErrReserved))
			}

			e.jsFuncs[k] = v
		}
	}
}

// Engine provides the templating engine.
type Engine struct {
	baseDir string
	readFS  fs.FS

	templator *template.Templator

	ran     bool
	jsFuncs map[string]func(call otto.FunctionCall) otto.Value
}

// New creates a new Engine with the provided options.
func New(opts ...Opt) *Engine {
	t := &template.Templator{
		// Reserving the names for now
		TmplFuncs: map[string]any{
			"templateFile":   nil,
			"templateString": nil,
		},
		WriteFunc: func(s string, b []byte) error {
			return os.WriteFile(s, b, os.ModePerm)
		},
	}

	e := &Engine{
		templator: t,
		jsFuncs:   map[string]func(call otto.FunctionCall) otto.Value{},
	}

	t.ReadFunc = e.readFile

	e.jsFuncs = map[string]func(call otto.FunctionCall) otto.Value{
		"require":              e.require,
		"templateFile":         e.templateFileJS,
		"templateString":       e.templateStringJS,
		"registerTemplateFunc": e.registerTemplateFunc,
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

	s, err := vm.Compile("", script)
	if err != nil {
		return fmt.Errorf("failed to compile script: %w", err)
	}

	if _, err := vm.Run(s); err != nil {
		return fmt.Errorf("failed to run script: %w", err)
	}

	return nil
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

func (e *Engine) init(data any) (*otto.Otto, error) {
	if e.ran {
		return nil, ErrAlreadyRan
	}
	e.ran = true

	vm := otto.New()

	for k, v := range e.jsFuncs {
		if err := vm.Set(k, v); err != nil {
			return nil, fmt.Errorf("failed to set js function %s: %w", k, err)
		}
	}

	// This need to have the vm passed in so that the functions can be called
	e.templator.TmplFuncs["templateFile"] = func(vm *otto.Otto) func(string, string, any) (string, error) {
		return func(templateFile, outFile string, data any) (string, error) {
			err := e.templator.TemplateFile(vm, templateFile, outFile, data)
			if err != nil {
				return "", err
			}

			return "", nil
		}
	}(vm)
	e.templator.TmplFuncs["templateString"] = func(vm *otto.Otto) func(string, any) (string, error) {
		return func(templateFile string, data any) (string, error) {
			templated, err := e.templator.TemplateString(vm, templateFile, data)
			if err != nil {
				return "", err
			}

			return templated, nil
		}
	}(vm)

	e.templator.ContextData = data
	if err := vm.Set("context", &template.Context{
		Global: data,
		Local:  data,
	}); err != nil {
		return nil, fmt.Errorf("failed to set context: %w", err)
	}

	return vm, nil
}

func (e *Engine) require(call otto.FunctionCall) otto.Value {
	vm := call.Otto

	scriptPath := call.Argument(0).String()

	script, err := e.readFile(scriptPath)
	if err != nil {
		panic(vm.MakeCustomError("requireScript", err.Error()))
	}

	s, err := vm.Compile("", script)
	if err != nil {
		panic(vm.MakeCustomError("requireScript", err.Error()))
	}

	if _, err := vm.Run(s); err != nil {
		panic(vm.MakeCustomError("requireScript", err.Error()))
	}

	return otto.Value{}
}

func (e *Engine) registerTemplateFunc(call otto.FunctionCall) otto.Value {
	name := call.Argument(0).String()
	fn := call.Argument(1)

	if _, ok := e.templator.TmplFuncs[name]; ok {
		panic(call.Otto.MakeCustomError("registerTemplateFunc", fmt.Sprintf("template function %s already exists", name)))
	}

	e.templator.TmplFuncs[name] = func(fn otto.Value) func(args ...interface{}) any {
		return func(args ...interface{}) any {
			val, err := fn.Call(fn, args...)
			if err != nil {
				panic(err)
			}

			v, err := val.Export()
			if err != nil {
				panic(err)
			}

			return v
		}
	}(fn)

	return otto.Value{}
}

func (e *Engine) readFile(file string) ([]byte, error) {
	filePath := file
	if e.baseDir != "" {
		filePath = path.Join(e.baseDir, filePath)
	}

	if e.readFS != nil {
		return fs.ReadFile(e.readFS, filePath)
	}
	return os.ReadFile(filePath)
}
