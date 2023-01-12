package easytemplate

import (
	"fmt"
	"os"
	"path"
	"reflect"
	"runtime"
	"strings"
	"text/template"

	"github.com/robertkrimen/otto"
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
			e.tmplFuncs[k] = v
		}
	}
}

type Engine struct {
	templateDir string
	scriptDir   string
	ran         bool
	tmplFuncs   template.FuncMap
	contextData interface{}
}

func New(opts ...Opt) *Engine {
	return &Engine{
		tmplFuncs: template.FuncMap{},
	}
}

// TODO: return useful errors
// TODO: Allow setting filesystem for embedded files
func (e *Engine) RunScript(scriptFile string, data any) error {
	if e.ran {
		return fmt.Errorf("the templating engine can only be run once, create a new instance to run again")
	}
	e.ran = true

	vm := otto.New()

	if err := registerFuncs(vm, []func(call otto.FunctionCall) otto.Value{
		e.require,
		e.templateFile,
		e.templateString,
		e.registerTemplateFunc,
	}); err != nil {
		return err
	}

	e.contextData = data
	if err := vm.Set("context", data); err != nil {
		return err
	}

	s, err := vm.Compile(scriptFile, nil)
	if err != nil {
		return err
	}

	if _, err := vm.Run(s); err != nil {
		return err
	}

	return nil
}

func (e *Engine) RunTemplate(templateFile string, outFile string, data any) error {
	if e.ran {
		return fmt.Errorf("the templating engine can only be run once, create a new instance to run again")
	}
	e.ran = true

	vm := otto.New()

	if err := registerFuncs(vm, []func(call otto.FunctionCall) otto.Value{
		e.require,
		e.templateFile,
		e.templateString,
		e.registerTemplateFunc,
	}); err != nil {
		return err
	}

	e.contextData = data
	if err := vm.Set("context", data); err != nil {
		return err
	}

	templated, err := e.tmpl(vm, templateFile, data)
	if err != nil {
		return err
	}

	if err := os.WriteFile(outFile, []byte(templated), 0o644); err != nil {
		return err
	}

	return nil
}

func registerFuncs(vm *otto.Otto, funcs []func(call otto.FunctionCall) otto.Value) error {
	for _, fn := range funcs {
		if err := registerFunc(vm, fn); err != nil {
			return err
		}
	}

	return nil
}

func registerFunc(vm *otto.Otto, fn func(call otto.FunctionCall) otto.Value) error {
	name := runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
	parts := strings.Split(name, ".")

	funcName := parts[len(parts)-1]
	funcName = strings.TrimSuffix(funcName, "-fm")

	if err := vm.Set(funcName, fn); err != nil {
		return err
	}

	return nil
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
