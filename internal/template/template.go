// Package template contains methods that will template go text/templates files that may contain sjs snippets.
package template

//go:generate mockgen -destination=./mocks/template_mock.go -package mocks github.com/speakeasy-api/easytemplate/internal/template VM
//go:generate mockgen -destination=./mocks/fileinfo.go -package mocks io/fs FileInfo

import (
	"bytes"
	"fmt"
	"io/fs"
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
	// Deprecated: Use WriteFileFunc instead.
	WriteFunc func(string, []byte) error
	// WriteFileFunc represents a function that writes a file.
	WriteFileFunc func(string, []byte, fs.FileMode) error
	// ReadFunc represents a function that reads a file.
	ReadFunc func(string) ([]byte, error)
	// StatFunc can stat a file to determine its attributes.
	StatFunc func(string) (fs.FileInfo, error)
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
	// WriteFunc is the WriteFunc to use. Deprecated: Use WriteFileFunc instead.
	WriteFunc      WriteFunc
	WriteFileFunc  WriteFileFunc
	ReadFunc       ReadFunc
	StatFunc       StatFunc
	TmplFuncs      map[string]any
	contextData    any
	globalComputed goja.Value
}

// SetContextData allows the setting of global context for templating.
func (t *Templator) SetContextData(contextData any, globalComputed goja.Value) {
	t.contextData = contextData
	t.globalComputed = globalComputed
}

// GetFileMode returns the file mode of the given file, if a StatFunc and WriteFileFunc have been provided (both are
// checked because even if you provide StatFunc, but no WriteFileFunc, the returned value will be ignored). Otherwise,
// it falls back to 0644. This permission is used for determining which file mode to give files in TemplateFile.
func (t *Templator) GetFileMode(name string) (fs.FileMode, error) {
	if t.StatFunc != nil && t.WriteFileFunc != nil {
		// TODO
		info, err := t.StatFunc(name)
		if err != nil {
			return 0, err
		}

		return info.Mode() & fs.ModePerm, nil
	}
	// fallback to default
	return 0644, nil
}

// WriteFile writes out the file using the WriteFileFunc. If no WriteFileFunc is provided, it will fall back to
// WriteFunc.
func (t *Templator) WriteFile(name string, data []byte, perm fs.FileMode) error {
	if t.WriteFileFunc != nil {
		return t.WriteFileFunc(name, data, perm)
	} else {
		return t.WriteFunc(name, data)
	}
}

// TemplateFile will template a file and write the output to outFile.
func (t *Templator) TemplateFile(vm VM, templateFile, outFile string, inputData any) error {
	output, err := t.TemplateString(vm, templateFile, inputData)
	if err != nil {
		return err
	}

	perm, err := t.GetFileMode(templateFile)
	if err != nil {
		return err
	}

	if err := t.WriteFile(outFile, []byte(output), perm); err != nil {
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

	return t.TemplateStringInput(vm, templatePath, string(data), inputData)
}

// TemplateStringInput will template the provided input string and return the output as a string.
func (t *Templator) TemplateStringInput(vm VM, name string, input string, inputData any) (out string, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("failed to render template: %s", e)
		}
	}()

	localComputed, err := vm.Run("localCreateComputedContextObject", `createComputedContextObject();`)
	if err != nil {
		return "", utils.HandleJSError("failed to create local computed context", err)
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
