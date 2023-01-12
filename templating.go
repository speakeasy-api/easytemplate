package easytemplate

import (
	"bytes"
	"os"
	"path"
	"regexp"
	"strings"
	"text/template"

	"github.com/robertkrimen/otto"
)

var sjsRegex = regexp.MustCompile("(?ms)(^```sjs\\s+(.*?)^sjs```)")

type templateContext struct {
	Global any
	Local  any
}

func (e *Engine) templateFile(call otto.FunctionCall) otto.Value {
	inputData, err := call.Argument(2).Export()
	if err != nil {
		panic(call.Otto.MakeCustomError("templateFile", err.Error()))
	}

	output, err := e.tmpl(call.Otto, call.Argument(0).String(), inputData)
	if err != nil {
		panic(call.Otto.MakeCustomError("templateFile", err.Error()))
	}

	if err := os.WriteFile(call.Argument(1).String(), []byte(output), 0o644); err != nil {
		panic(call.Otto.MakeCustomError("templateFile", err.Error()))
	}

	return otto.Value{}
}

func (e *Engine) templateString(call otto.FunctionCall) otto.Value {
	inputData, err := call.Argument(1).Export()
	if err != nil {
		panic(call.Otto.MakeCustomError("templateString", err.Error()))
	}

	output, err := e.tmpl(call.Otto, call.Argument(0).String(), inputData)
	if err != nil {
		panic(call.Otto.MakeCustomError("templateString", err.Error()))
	}

	val, err := otto.ToValue(output)
	if err != nil {
		panic(call.Otto.MakeCustomError("templateString", err.Error()))
	}

	return val
}

type inlineScriptContext struct {
	renderedContent []string
}

func newInlineScriptContext() *inlineScriptContext {
	return &inlineScriptContext{
		renderedContent: []string{},
	}
}

func (c *inlineScriptContext) render(call otto.FunctionCall) otto.Value {
	c.renderedContent = append(c.renderedContent, call.Argument(0).String())

	return otto.Value{}
}

func (e *Engine) tmpl(vm *otto.Otto, templatePath string, inputData any) (string, error) {
	tp := templatePath
	if e.templateDir != "" {
		tp = path.Join(e.templateDir, templatePath)
	}

	data, err := os.ReadFile(tp)
	if err != nil {
		return "", err
	}

	evaluated, err := replaceAllStringSubmatchFunc(sjsRegex, string(data), func(match []string) (string, error) {
		if len(match) != 3 {
			return match[0], nil
		}

		s, err := vm.Compile("", match[2])
		if err != nil {
			return "", err
		}

		c := newInlineScriptContext()
		if err := registerFunc(vm, c.render); err != nil {
			return "", err
		}

		if _, err := vm.Run(s); err != nil {
			return "", err
		}

		if err := vm.Set("render", otto.UndefinedValue()); err != nil {
			return "", err
		}

		return strings.Join(c.renderedContent, "\n"), nil
	})
	if err != nil {
		return "", err
	}

	t, err := template.New(templatePath).Funcs(e.tmplFuncs).Parse(evaluated)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer

	if err := t.Execute(&buf, templateContext{
		Global: e.contextData,
		Local:  inputData,
	}); err != nil {
		return "", err
	}

	return buf.String(), nil
}
