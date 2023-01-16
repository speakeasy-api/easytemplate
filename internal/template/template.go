// Package template contains methods that will template go text/templates files that may contain sjs snippets.
package template

//go:generate mockgen -destination=./mocks/template_mock.go -package mocks github.com/speakeasy-api/easytemplate/internal/template VM

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"text/template"

	"github.com/robertkrimen/otto"
	"github.com/speakeasy-api/easytemplate/internal/utils"
)

type (
	// WriteFunc represents a function that writes a file.
	WriteFunc func(string, []byte) error
	// ReadFunc represents a function that reads a file.
	ReadFunc func(string) ([]byte, error)
)

var sjsRegex = regexp.MustCompile("(?ms)(^```sjs\\s+(.*?)^sjs```)")

// Context is the context that is passed templates or js.
type Context struct {
	Global any
	Local  any
}

// VM represents a virtual machine that can be used to run js.
type VM interface {
	Get(name string) (otto.Value, error)
	Set(name string, value interface{}) error
	Compile(filename string, src interface{}) (*otto.Script, error)
	Run(src interface{}) (otto.Value, error)
}

// Templator extends the go text/template package to allow for sjs snippets.
type Templator struct {
	WriteFunc   WriteFunc
	ReadFunc    ReadFunc
	ContextData interface{}
	TmplFuncs   map[string]any
}

// TemplateFile will template a file and write the output to outFile.
func (t *Templator) TemplateFile(vm VM, templateFile, outFile string, inputData any) error {
	output, err := t.TemplateString(vm, templateFile, inputData)
	if err != nil {
		return err
	}

	if err := t.WriteFunc(outFile, []byte(output)); err != nil {
		return fmt.Errorf("failed to write file %s: %w", outFile, err)
	}

	return nil
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

// TemplateString will template a string and return the output.
func (t *Templator) TemplateString(vm VM, templatePath string, inputData any) (out string, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("failed to render template: %s", e)
		}
	}()

	context := &Context{
		Global: t.ContextData,
		Local:  inputData,
	}

	currentContext, err := vm.Get("context")
	if err != nil {
		return "", fmt.Errorf("failed to get context: %w", err)
	}

	if err := vm.Set("context", context); err != nil {
		return "", fmt.Errorf("failed to set context: %w", err)
	}

	data, err := t.ReadFunc(templatePath)
	if err != nil {
		return "", fmt.Errorf("failed to read template file: %w", err)
	}

	evaluated, err := utils.ReplaceAllStringSubmatchFunc(sjsRegex, string(data), func(match []string) (string, error) {
		const expectedMatchLen = 3
		if len(match) != expectedMatchLen {
			return match[0], nil
		}

		return t.execSJSBlock(vm, match[2])
	})
	if err != nil {
		return "", err
	}

	// Use the local context from the inline script
	localContext, err := getLocalContext(vm)
	if err != nil {
		return "", err
	}
	context.Local = localContext

	out, err = t.execTemplate(templatePath, evaluated, context)
	if err != nil {
		return "", err
	}

	// Reset the context back to the previous one
	if err := vm.Set("context", currentContext); err != nil {
		return "", fmt.Errorf("failed to reset context: %w", err)
	}

	return out, nil
}

func (t *Templator) execSJSBlock(vm VM, js string) (string, error) {
	s, err := vm.Compile("", js)
	if err != nil {
		return "", fmt.Errorf("failed to compile inline script: %w", err)
	}

	currentRender, err := vm.Get("render")
	if err != nil {
		return "", fmt.Errorf("failed to get render function: %w", err)
	}

	c := newInlineScriptContext()
	if err := vm.Set("render", c.render); err != nil {
		return "", fmt.Errorf("failed to set render function: %w", err)
	}

	if _, err := vm.Run(s); err != nil {
		return "", fmt.Errorf("failed to run inline script: %w", err)
	}

	if err := vm.Set("render", currentRender); err != nil {
		return "", fmt.Errorf("failed to unset render function: %w", err)
	}

	return strings.Join(c.renderedContent, "\n"), nil
}

func getLocalContext(vm VM) (any, error) {
	// Get the local context back as it might have been modified by the inline script
	contextVal, err := vm.Get("context")
	if err != nil {
		return nil, fmt.Errorf("failed to get context: %w", err)
	}

	localContextVal, err := contextVal.Object().Get("Local")
	if err != nil {
		return nil, fmt.Errorf("failed to get Local attribute: %w", err)
	}

	localContext, err := localContextVal.Export()
	if err != nil {
		return nil, fmt.Errorf("failed to export local context: %w", err)
	}

	return localContext, nil
}

func (t *Templator) execTemplate(name string, tmplContent string, data any) (string, error) {
	tmp, err := template.New(name).Funcs(t.TmplFuncs).Parse(tmplContent)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer

	if err := tmp.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}
