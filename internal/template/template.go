// Package template contains methods that will template go text/templates files that may contain sjs snippets.
package template

//go:generate mockgen -destination=./mocks/template_mock.go -package mocks github.com/speakeasy-api/easytemplate/internal/template VM

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"text/template"

	"github.com/dop251/goja"
	"github.com/speakeasy-api/easytemplate/internal/utils"
	"github.com/speakeasy-api/easytemplate/internal/vm"
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
	Run(name string, src string, opts ...vm.Option) (goja.Value, error)
	ToObject(val goja.Value) *goja.Object
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

// TemplateFileMultiple will template the provided file numTimes and write the output to outFile.
func (t *Templator) TemplateFileMultiple(vm VM, templateFile, outFile string, inputData any, numTimes int) error {
	output, err := t.TemplateStringMultiple(vm, templateFile, inputData, numTimes)
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

// TemplateString will template the provided file and return the output as a string.
func (t *Templator) TemplateString(vm VM, templatePath string, inputData any) (out string, err error) {
	data, err := t.ReadFunc(templatePath)
	if err != nil {
		return "", fmt.Errorf("failed to read template file: %w", err)
	}

	return t.TemplateStringInput(vm, templatePath, string(data), inputData, 1)
}

// TemplateStringMultiple will template the provided file numTimes and return the output as a string.
func (t *Templator) TemplateStringMultiple(vm VM, templatePath string, inputData any, numTimes int) (out string, err error) {
	data, err := t.ReadFunc(templatePath)
	if err != nil {
		return "", fmt.Errorf("failed to read template file: %w", err)
	}

	return t.TemplateStringInput(vm, templatePath, string(data), inputData, numTimes)
}

// TemplateStringInput will template the provided input string and return the output as a string.
func (t *Templator) TemplateStringInput(vm VM, name string, input string, inputData any, numTimes int) (out string, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("failed to render template: %s", e)
		}
	}()

	localComputed, err := vm.Run("localCreateComputedContextObject", `createComputedContextObject();`)
	if err != nil {
		return "", utils.HandleJSError("failed to create local computed context", err)
	}

	currentContext := vm.Get("context")

	for i := 0; i < numTimes; i++ {
		context := &Context{
			Global:         t.contextData,
			GlobalComputed: t.globalComputed,
			Local:          inputData,
			LocalComputed:  localComputed,
		}

		if err := vm.Set("context", context); err != nil {
			return "", fmt.Errorf("failed to set context: %w", err)
		}

		evaluated, replacedLines, err := t.evaluateInlineScripts(vm, name, input)
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

		out, err = t.execTemplate(name, evaluated, tmplCtx, replacedLines)
		if err != nil {
			return "", err
		}

		// Set the output as the input for the next iteration and update the computed context
		input = out
		if i == 0 {
			input, numTimes = t.applyRecurseCanary(input, numTimes)
		}
		localComputed = getComputedContext(vm)
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

		output, err := t.execSJSBlock(vm, match[2], templatePath, findJSBlockLineNumber(content, match[2]))
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

func (t *Templator) execSJSBlock(v VM, js, templatePath string, jsBlockLineNumber int) (string, error) {
	currentRender := v.Get("render")

	c := newInlineScriptContext()
	if err := v.Set("render", c.render); err != nil {
		return "", fmt.Errorf("failed to set render function: %w", err)
	}

	if _, err := v.Run(templatePath, js, vm.WithStartingLineNumber(jsBlockLineNumber)); err != nil {
		return "", fmt.Errorf("failed to run inline script in %s:\n```sjs\n%ssjs```\n%w", templatePath, js, err)
	}

	if err := v.Set("render", currentRender); err != nil {
		return "", fmt.Errorf("failed to unset render function: %w", err)
	}

	return strings.Join(c.renderedContent, "\n"), nil
}

func getComputedContext(vm VM) goja.Value {
	// Get the local context back as it might have been modified by the inline script
	contextVal := vm.Get("context")

	computedVal := vm.ToObject(contextVal).Get("LocalComputed")

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

func recurseCanary() []string {
	return []string{
		"5f9c88133f5cc60a84009b841192a2d8f83dd901e8112fe77284e461b4039ccd",
		"f34e13e176147bebc0e4d644fd45d5727462c3b574b3ffc00df1b4683c15e54c",
		"6f6fd98465b1ef727f0692a3d08f70cb4b94ab987a97d0ee206d3d33d2032887",
		"f1cd9fe131302d6f37b17084fa8b7679debb6bff79853171e439483bbc2cb846",
		"2290a2c385c9f40a2af9f56095630ab8b80e067cdf8fc92beef93c2b16c2b8aa",
	}
}

// Recurse will let the engine know how many times the template should execute.
func (t *Templator) Recurse(vm VM, numTimes int) (out string, err error) {
	if numTimes < 1 || numTimes > len(recurseCanary()) {
		return "", fmt.Errorf("recurse(%v) invalid: %v outside bounds 1..%v", numTimes, numTimes, len(recurseCanary()))
	}

	return recurseCanary()[numTimes-1], nil
}

func (t *Templator) applyRecurseCanary(input string, execCount int) (string, int) {
	canaryList := recurseCanary()
	const recurseArgumentToExecCount = 2
	for i, canary := range canaryList {
		if strings.Contains(input, canary) {
			// recurse 1 means canary[0] is found, and execCount is now 2
			// if more than 1 "recurse" invocation in template, use the largest
			execCount = int(math.Max(float64(execCount), float64(i+recurseArgumentToExecCount)))
		}
		input = strings.ReplaceAll(input, canary, "")
	}

	return input, execCount
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

			return strings.Replace(matches[0], matches[1], strconv.Itoa(currentLineNumber+replacedLines), 1), nil
		})
		if rErr == nil {
			err = fmt.Errorf(errMsg)
		}
	}

	return err
}

func findJSBlockLineNumber(content string, block string) int {
	const replacement = "~-~BLOCK_REPLACEMENT~-~"

	content = strings.Replace(content, block, replacement, 1)

	lines := strings.Split(content, "\n")

	for i, line := range lines {
		if strings.Contains(line, replacement) {
			return i + 1
		}
	}

	return 0
}
