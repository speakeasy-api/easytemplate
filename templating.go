package easytemplate

import (
	"bytes"
	"fmt"
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

func (e *Engine) templateFileJS(call otto.FunctionCall) otto.Value {
	inputData, err := call.Argument(2).Export()
	if err != nil {
		panic(call.Otto.MakeCustomError("templateFile", err.Error()))
	}

	if err := e.templateFile(call.Otto, call.Argument(0).String(), call.Argument(1).String(), inputData); err != nil {
		panic(call.Otto.MakeCustomError("templateFile", err.Error()))
	}

	return otto.Value{}
}

func (e *Engine) templateFile(vm *otto.Otto, templateFile, outFile string, inputData any) error {
	output, err := e.tmpl(vm, templateFile, inputData)
	if err != nil {
		return err
	}

	if err := os.WriteFile(outFile, []byte(output), 0o644); err != nil {
		return err
	}

	return nil
}

func (e *Engine) templateStringJS(call otto.FunctionCall) otto.Value {
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

func (e *Engine) tmpl(vm *otto.Otto, templatePath string, inputData any) (out string, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("failed to render template: %s", e)
		}
	}()

	context := templateContext{
		Global: e.contextData,
		Local:  inputData,
	}

	tp := templatePath
	if e.templateDir != "" {
		tp = path.Join(e.templateDir, templatePath)
	}

	data, err := os.ReadFile(tp)
	if err != nil {
		return "", fmt.Errorf("failed to read template file: %w", err)
	}

	evaluated, err := replaceAllStringSubmatchFunc(sjsRegex, string(data), func(match []string) (string, error) {
		if len(match) != 3 {
			return match[0], nil
		}

		s, err := vm.Compile("", match[2])
		if err != nil {
			return "", fmt.Errorf("failed to compile inline script: %w", err)
		}

		c := newInlineScriptContext()
		if err := vm.Set("render", c.render); err != nil {
			return "", fmt.Errorf("failed to set render function: %w", err)
		}

		if err := vm.Set("context", context); err != nil {
			return "", fmt.Errorf("failed to set context: %w", err)
		}

		if _, err := vm.Run(s); err != nil {
			return "", fmt.Errorf("failed to run inline script: %w", err)
		}

		if err := vm.Set("render", otto.UndefinedValue()); err != nil {
			return "", fmt.Errorf("failed to unset render function: %w", err)
		}

		return strings.Join(c.renderedContent, "\n"), nil
	})
	if err != nil {
		return "", err
	}

	contextVal, err := vm.Get("context")
	if err != nil {
		return "", fmt.Errorf("failed to get local context: %w", err)
	}

	localContextVal, err := contextVal.Object().Get("Local")
	if err != nil {
		return "", fmt.Errorf("failed to get local context: %w", err)
	}

	localContext, err := localContextVal.Export()
	if err != nil {
		return "", fmt.Errorf("failed to export local context: %w", err)
	}
	context.Local = localContext

	t, err := template.New(templatePath).Funcs(e.tmplFuncs).Parse(evaluated)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer

	if err := t.Execute(&buf, context); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}
