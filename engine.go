package easytemplate

import (
	"fmt"
	"os"
	"path"
	"text/template"

	"github.com/robertkrimen/otto"
	_ "github.com/robertkrimen/otto/underscore"
)

type Opt func(*Engine)

func WithTemplateDir(templateDir string) Opt {
	return func(e *Engine) {
		e.templateDir = templateDir
	}
}

func WithScriptDir(scriptDir string) Opt {
	return func(e *Engine) {
		e.scriptDir = scriptDir
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
	templateDir string
	scriptDir   string
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

	s, err := vm.Compile(scriptFile, nil)
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

	templated, err := e.tmpl(vm, templateFile, data)
	if err != nil {
		return fmt.Errorf("failed to template file: %w", err)
	}

	if err := os.WriteFile(outFile, []byte(templated), 0o644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
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

	if e.scriptDir != "" {
		scriptPath = path.Join(e.scriptDir, scriptPath)
	}

	s, err := vm.Compile(scriptPath, nil)
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
