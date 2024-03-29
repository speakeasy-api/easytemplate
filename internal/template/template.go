// Package template contains methods that will template go text/templates files that may contain sjs snippets.
package template

//go:generate mockgen -destination=./mocks/template_mock.go -package mocks github.com/speakeasy-api/easytemplate/internal/template VM

import (
	"bytes"
	"context"
	"fmt"
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
	Global            any
	Local             any
	GlobalComputed    goja.Value
	LocalComputed     goja.Value
	RecursiveComputed goja.Value
}

type tmplContext struct {
	Global            any
	Local             any
	GlobalComputed    any
	LocalComputed     any
	RecursiveComputed any
}

// VM represents a virtual machine that can be used to run js.
type VM interface {
	Get(name string) goja.Value
	Set(name string, value any) error
	Run(ctx context.Context, name string, src string, opts ...vm.Option) (goja.Value, error)
	ToObject(val goja.Value) *goja.Object
}

// Templator extends the go text/template package to allow for sjs snippets.
type Templator struct {
	WriteFunc      WriteFunc
	ReadFunc       ReadFunc
	TmplFuncs      map[string]any
	Debug          bool
	contextData    any
	globalComputed goja.Value
}

// SetContextData allows the setting of global context for templating.
func (t *Templator) SetContextData(contextData any, globalComputed goja.Value) {
	t.contextData = contextData
	t.globalComputed = globalComputed
}

// TemplateFile will template a file and write the output to outFile.
func (t *Templator) TemplateFile(ctx context.Context, vm VM, templateFile, outFile string, inputData any) error {
	output, err := t.TemplateString(ctx, vm, templateFile, inputData)
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
func (t *Templator) TemplateString(ctx context.Context, vm VM, templatePath string, inputData any) (out string, err error) {
	data, err := t.ReadFunc(templatePath)
	if err != nil {
		return "", fmt.Errorf("failed to read template file: %w", err)
	}

	return t.TemplateStringInput(ctx, vm, templatePath, string(data), inputData)
}

// TemplateStringInput will template the provided input string and return the output as a string.
//
//nolint:funlen
func (t *Templator) TemplateStringInput(ctx context.Context, vm VM, name string, input string, inputData any) (out string, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("failed to render template: %s", e)
		}
	}()

	localComputed, err := vm.Run(ctx, "localCreateComputedContextObject", `createComputedContextObject();`)
	if err != nil {
		return "", utils.HandleJSError("failed to create local computed context", err)
	}

	currentContext := vm.Get("context")

	currentRecursiveComputed := getRecursiveComputedContext(vm)
	localRecursiveComputed := currentRecursiveComputed

	numIterations := 1
	numRecursions, err := t.isRecursiveTemplate(input)
	if err != nil {
		return "", err
	}
	if numRecursions > 0 {
		numIterations = numRecursions + 1
		localRecursiveComputed, err = vm.Run(ctx, "recursiveCreateComputedContextObject", `createComputedContextObject();`)
		if err != nil {
			return "", utils.HandleJSError("failed to create recursive computed context", err)
		}
	}

	for i := 0; i < numIterations; i++ {
		context := &Context{
			Global:            t.contextData,
			GlobalComputed:    t.globalComputed,
			Local:             inputData,
			LocalComputed:     localComputed,
			RecursiveComputed: localRecursiveComputed,
		}

		if err := vm.Set("context", context); err != nil {
			return "", fmt.Errorf("failed to set context: %w", err)
		}

		evaluated, replacedLines, err := t.evaluateInlineScripts(ctx, vm, name, input)
		if err != nil {
			return "", err
		}

		// Get the computed context back as it might have been modified by the inline script
		localComputed = getLocalComputedContext(vm)

		tmplCtx := &tmplContext{
			Global:            context.Global,
			Local:             context.Local,
			GlobalComputed:    context.GlobalComputed.Export(),
			LocalComputed:     localComputed.Export(),
			RecursiveComputed: localRecursiveComputed.Export(),
		}

		out, err = t.execTemplate(name, evaluated, tmplCtx, replacedLines)
		if err != nil {
			return "", err
		}

		// Set the output as the input for the next iteration and update the computed context
		var cont bool
		input, cont, err = t.applyRecurseCanary(out)
		if err != nil {
			return "", err
		}
		// If the output is the same as the input, or no canary found, we don't need to continue
		if !cont || evaluated == out {
			break
		}
		localRecursiveComputed = getRecursiveComputedContext(vm)
	}

	// Reset the context back to the previous one
	if err := vm.Set("context", currentContext); err != nil {
		return "", fmt.Errorf("failed to reset context: %w", err)
	}

	return out, nil
}

func (t *Templator) evaluateInlineScripts(ctx context.Context, vm VM, templatePath, content string) (string, int, error) {
	replacedLines := 0

	evaluated, err := utils.ReplaceAllStringSubmatchFunc(sjsRegex, content, func(match []string) (string, error) {
		const expectedMatchLen = 3
		if len(match) != expectedMatchLen {
			return match[0], nil
		}

		output, err := t.execSJSBlock(ctx, vm, match[2], templatePath, findJSBlockLineNumber(content, match[2]))
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

func (t *Templator) execSJSBlock(ctx context.Context, v VM, js, templatePath string, jsBlockLineNumber int) (string, error) {
	currentRender := v.Get("render")

	c := newInlineScriptContext()
	if err := v.Set("render", c.render); err != nil {
		return "", fmt.Errorf("failed to set render function: %w", err)
	}

	if _, err := v.Run(ctx, templatePath, js, vm.WithStartingLineNumber(jsBlockLineNumber)); err != nil {
		return "", fmt.Errorf("failed to run inline script in %s:\n```sjs\n%ssjs```\n%w", templatePath, js, err)
	}

	if err := v.Set("render", currentRender); err != nil {
		return "", fmt.Errorf("failed to unset render function: %w", err)
	}

	return strings.Join(c.renderedContent, "\n"), nil
}

func getLocalComputedContext(vm VM) goja.Value {
	// Get the local context back as it might have been modified by the inline script
	contextVal := vm.Get("context")

	computedVal := vm.ToObject(contextVal).Get("LocalComputed")

	return computedVal
}

func getRecursiveComputedContext(vm VM) goja.Value {
	contextVal := vm.Get("context")
	if contextVal == goja.Undefined() {
		return goja.Undefined()
	}

	computedVal := vm.ToObject(contextVal).Get("RecursiveComputed")

	return computedVal
}

func (t *Templator) execTemplate(name string, tmplContent string, data any, replacedLines int) (string, error) {
	tmp, err := template.New(name).Funcs(t.TmplFuncs).Parse(tmplContent)
	if err != nil {
		if t.Debug {
			//nolint:forbidigo
			fmt.Println(tmplContent)
		}
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer

	if err := tmp.Execute(&buf, data); err != nil {
		err = adjustLineNumber(name, err, replacedLines)
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// Recurse will let the engine know how many times the template should execute.
func (t *Templator) Recurse(_ VM, numTimes int) (out string, err error) {
	if numTimes < 1 {
		return "", fmt.Errorf("recurse(%d) invalid: must recurse at least once", numTimes)
	}

	return fmt.Sprintf(canaryPlaceholder, strconv.Itoa(numTimes-1)), nil
}

var (
	canaryPlaceholder = "~-~SPEAKEASY_RECURSE_CANARY_%s~-~"
	canaryRegex       = regexp.MustCompile(fmt.Sprintf(canaryPlaceholder, `(\d+)`))
	recurseRegex      = regexp.MustCompile(`^{{-* *recurse (\d+) *-*}}$`)
)

func (t *Templator) isRecursiveTemplate(input string) (int, error) {
	lines := strings.Split(strings.ReplaceAll(input, "\r\n", "\n"), "\n")

	matches := recurseRegex.FindAllStringSubmatch(lines[0], -1)
	if len(matches) != 1 {
		return -1, nil
	}

	num, err := strconv.Atoi(matches[0][1])
	if err != nil {
		return 0, err
	}

	return num, nil
}

func (t *Templator) getCanaryNum(input string) (int, error) {
	matches := canaryRegex.FindAllStringSubmatch(input, -1)
	if len(matches) == 0 {
		return -1, nil
	}

	if len(matches) > 1 {
		return 0, fmt.Errorf("recurse canary found more than once in template. recurse should only be declared once per template")
	}

	num, err := strconv.Atoi(matches[0][1])
	if err != nil {
		return 0, err
	}

	return num, nil
}

func (t *Templator) applyRecurseCanary(input string) (string, bool, error) {
	num, err := t.getCanaryNum(input)
	if err != nil {
		return "", false, err
	}

	if num == -1 {
		return input, false, nil
	}

	replacementString := ""

	if num > 0 {
		replacementString = fmt.Sprintf(canaryPlaceholder, strconv.Itoa(num-1))
	}

	return strings.Replace(input, fmt.Sprintf(canaryPlaceholder, strconv.Itoa(num)), replacementString, 1), true, nil
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
