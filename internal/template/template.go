// Package template contains methods that will template go text/templates files that may contain sjs snippets.
package template

//go:generate mockgen -destination=./mocks/template_mock.go -package mocks github.com/speakeasy-api/easytemplate/internal/template VM

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"text/template"

	"github.com/dop251/goja"
	"github.com/speakeasy-api/easytemplate/internal/utils"
)

type (
	// WriteFunc represents a function that writes a file.
	WriteFunc func(string, []byte) error
	// ReadFunc represents a function that reads a file.
	ReadFunc func(string) ([]byte, error)
)

var sjsRegex = regexp.MustCompile("(?ms)(```sjs\\s*\\n*(.*?)sjs```)")

// Context is the context that is passed templates or js.
type Context struct {
	Global         any
	Local          any
	GlobalComputed goja.Value
	LocalComputed  goja.Value
}

type tmplContext struct {
	Global         any
	Local          any
	GlobalComputed any
	LocalComputed  any
}

// VM represents a virtual machine that can be used to run js.
type VM interface {
	Get(name string) goja.Value
	Set(name string, value any) error
	Compile(name string, src string, strict bool) (*goja.Program, error)
	RunProgram(p *goja.Program) (result goja.Value, err error)
	RunString(script string) (result goja.Value, err error)
	GetObject(val goja.Value) *goja.Object
}

// Templator extends the go text/template package to allow for sjs snippets.
type Templator struct {
	WriteFunc      WriteFunc
	ReadFunc       ReadFunc
	TmplFuncs      map[string]any
	contextData    any
	globalComputed goja.Value
}

// SetContextData allows the setting of global context for templating.
func (t *Templator) SetContextData(contextData any, globalComputed goja.Value) {
	t.contextData = contextData
	t.globalComputed = globalComputed
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

func (c *inlineScriptContext) render(call goja.FunctionCall) goja.Value {
	c.renderedContent = append(c.renderedContent, call.Argument(0).String())

	return goja.Undefined()
}

// TemplateString will template a string and return the output.
func (t *Templator) TemplateString(vm VM, templatePath string, inputData any) (out string, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("failed to render template: %s", e)
		}
	}()

	localComputed, err := vm.RunString(`createComputedContextObject();`)
	if err != nil {
		return "", fmt.Errorf("failed to create local computed context: %w", err)
	}

	context := &Context{
		Global:         t.contextData,
		GlobalComputed: t.globalComputed,
		Local:          inputData,
		LocalComputed:  localComputed,
	}

	currentContext := vm.Get("context")

	if err := vm.Set("context", context); err != nil {
		return "", fmt.Errorf("failed to set context: %w", err)
	}

	data, err := t.ReadFunc(templatePath)
	if err != nil {
		return "", fmt.Errorf("failed to read template file: %w", err)
	}

	evaluated, replacedLines, err := t.evaluateInlineScripts(vm, templatePath, string(data))
	if err != nil {
		return "", err
	}

	// Get the computed context back as it might have been modified by the inline script
	localComputed = getComputedContext(vm)

	tmplCtx := &tmplContext{
		Global:         context.Global,
		Local:          context.Local,
		GlobalComputed: context.GlobalComputed.Export(),
		LocalComputed:  localComputed.Export(),
	}

	out, err = t.execTemplate(templatePath, evaluated, tmplCtx, replacedLines)
	if err != nil {
		return "", err
	}

	// Reset the context back to the previous one
	if err := vm.Set("context", currentContext); err != nil {
		return "", fmt.Errorf("failed to reset context: %w", err)
	}

	return out, nil
}

func (t *Templator) evaluateInlineScripts(vm VM, templatePath, content string) (string, int, error) {
	replacedLines := 0

	evaluated, err := utils.ReplaceAllStringSubmatchFunc(sjsRegex, content, func(match []string) (string, error) {
		const expectedMatchLen = 3
		if len(match) != expectedMatchLen {
			return match[0], nil
		}

		output, err := t.execSJSBlock(vm, match[2], templatePath)
		if err != nil {
			return "", err
		}

		replacedLines += strings.Count(match[1], "\n") - strings.Count(output, "\n")

		return output, nil
	})
	if err != nil {
		return "", 0, err
	}

	return evaluated, replacedLines, nil
}

func (t *Templator) execSJSBlock(vm VM, js, templatePath string) (string, error) {
	s, err := vm.Compile("inline", js, true)
	if err != nil {
		return "", fmt.Errorf("failed to compile inline script: %w", err)
	}

	currentRender := vm.Get("render")

	c := newInlineScriptContext()
	if err := vm.Set("render", c.render); err != nil {
		return "", fmt.Errorf("failed to set render function: %w", err)
	}

	if _, err := vm.RunProgram(s); err != nil {
		return "", fmt.Errorf("failed to run inline script in %s:\n%s\n%w", templatePath, js, err)
	}

	if err := vm.Set("render", currentRender); err != nil {
		return "", fmt.Errorf("failed to unset render function: %w", err)
	}

	return strings.Join(c.renderedContent, "\n"), nil
}

func getComputedContext(vm VM) goja.Value {
	// Get the local context back as it might have been modified by the inline script
	contextVal := vm.Get("context")

	computedVal := vm.GetObject(contextVal).Get("LocalComputed")

	return computedVal
}

func (t *Templator) execTemplate(name string, tmplContent string, data any, replacedLines int) (string, error) {
	tmp, err := template.New(name).Funcs(t.TmplFuncs).Parse(tmplContent)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer

	if err := tmp.Execute(&buf, data); err != nil {
		err = adjustLineNumber(name, err, replacedLines)
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

func adjustLineNumber(name string, err error, replacedLines int) error {
	lineNumRegex, rErr := regexp.Compile(fmt.Sprintf(`template: %s:(\d+)`, regexp.QuoteMeta(name)))
	if rErr == nil {
		errMsg, rErr := utils.ReplaceAllStringSubmatchFunc(lineNumRegex, err.Error(), func(matches []string) (string, error) {
			if len(matches) != 2 { //nolint:gomnd
				return matches[0], nil
			}

			currentLineNumber, err := strconv.Atoi(matches[1])
			if err != nil {
				return matches[0], nil //nolint:nilerr
			}

			return strings.Replace(matches[0], matches[1], fmt.Sprintf("%d", currentLineNumber+replacedLines), 1), nil
		})
		if rErr == nil {
			err = fmt.Errorf(errMsg)
		}
	}

	return err
}
