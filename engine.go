package easytemplate

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"text/template"

	"github.com/robertkrimen/otto"
	_ "github.com/robertkrimen/otto/underscore"
)

type WriteFunc func(string, []byte) error

type Opt func(*Engine)

func WithBaseDir(baseDir string) Opt {
	return func(e *Engine) {
		e.baseDir = baseDir
	}
}

func WithReadFileSystem(fs fs.FS) Opt {
	return func(e *Engine) {
		e.readFS = fs
	}
}

func WithWriteFunc(writeFunc WriteFunc) Opt {
	return func(e *Engine) {
		e.writeFunc = writeFunc
	}
}

func WithTemplateFuncs(funcs template.FuncMap) Opt {
	return func(e *Engine) {
		for k, v := range funcs {
			if _, ok := e.tmplFuncs[k]; ok {
				panic(fmt.Errorf("template function %s is a reserved function and can't be overridden", k))
			}

			e.tmplFuncs[k] = v
		}
	}
}

func WithJSFuncs(funcs map[string]func(call otto.FunctionCall) otto.Value) Opt {
	return func(e *Engine) {
		for k, v := range funcs {
			if _, ok := e.jsFuncs[k]; ok {
				panic(fmt.Errorf("js function %s is a reserved function and can't be overridden", k))
			}

			e.jsFuncs[k] = v
		}
	}
}

type Engine struct {
	baseDir     string
	readFS      fs.FS
	writeFunc   WriteFunc
	ran         bool
	tmplFuncs   template.FuncMap
	jsFuncs     map[string]func(call otto.FunctionCall) otto.Value
	contextData interface{}
}

func New(opts ...Opt) *Engine {
	e := &Engine{
		// Reserving the names for now
		tmplFuncs: template.FuncMap{
			"templateFile":   nil,
			"templateString": nil,
		},
		jsFuncs: map[string]func(call otto.FunctionCall) otto.Value{},
	}

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

// TODO: Allow setting filesystem for embedded files
func (e *Engine) RunScript(scriptFile string, data any) error {
	vm, err := e.init(data)
	if err != nil {
		return err
	}

	script, err := e.ReadFile(scriptFile)
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

func (e *Engine) RunTemplate(templateFile string, outFile string, data any) error {
	vm, err := e.init(data)
	if err != nil {
		return err
	}

	return e.templateFile(vm, templateFile, outFile, data)
}

func (e *Engine) RunTemplateString(templateString string, data any) (string, error) {
	vm, err := e.init(data)
	if err != nil {
		return "", err
	}

	return e.tmpl(vm, templateString, data)
}

func (e *Engine) init(data any) (*otto.Otto, error) {
	if e.ran {
		return nil, fmt.Errorf("the templating engine can only be run once, create a new instance to run again")
	}
	e.ran = true

	vm := otto.New()

	for k, v := range e.jsFuncs {
		if err := vm.Set(k, v); err != nil {
			return nil, fmt.Errorf("failed to set js function %s: %w", k, err)
		}
	}

	// This need to have the vm passed in so that the functions can be called
	e.tmplFuncs["templateFile"] = func(vm *otto.Otto) func(string, string, any) (string, error) {
		return func(templateFile, outFile string, data any) (string, error) {
			err := e.templateFile(vm, templateFile, outFile, data)
			if err != nil {
				return "", err
			}

			return "", nil
		}
	}(vm)
	e.tmplFuncs["templateString"] = func(vm *otto.Otto) func(string, any) (string, error) {
		return func(templateFile string, data any) (string, error) {
			templated, err := e.tmpl(vm, templateFile, data)
			if err != nil {
				return "", err
			}

			return templated, nil
		}
	}(vm)

	e.contextData = data
	if err := vm.Set("context", templateContext{
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

	script, err := e.ReadFile(scriptPath)
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

	if _, ok := e.tmplFuncs[name]; ok {
		panic(call.Otto.MakeCustomError("registerTemplateFunc", fmt.Sprintf("template function %s already exists", name)))
	}

	e.tmplFuncs[name] = func(fn otto.Value) func(args ...interface{}) any {
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

func (e *Engine) ReadFile(file string) ([]byte, error) {
	filePath := file
	if e.baseDir != "" {
		filePath = path.Join(e.baseDir, filePath)
	}

	if e.readFS != nil {
		return fs.ReadFile(e.readFS, filePath)
	} else {
		return os.ReadFile(filePath)
	}
}
