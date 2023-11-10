package easytemplate

import (
	"github.com/dop251/goja"
)

func (e *Engine) templateFileJS(call CallContext) goja.Value {
	inputData := call.Argument(2).Export() //nolint:gomnd

	if err := e.templator.TemplateFile(call.VM, call.Argument(0).String(), call.Argument(1).String(), inputData); err != nil {
		panic(call.VM.NewGoError(err))
	}

	return goja.Undefined()
}

//nolint:gomnd
func (e *Engine) templateFileMultipleJS(call CallContext) goja.Value {
	templateFile := call.Argument(0).String()
	outFile := call.Argument(1).String()
	inputData := call.Argument(2).Export()
	numTimes := call.Argument(3).ToInteger()

	if err := e.templator.TemplateFileMultiple(call.VM, templateFile, outFile, inputData, int(numTimes)); err != nil {
		panic(call.VM.NewGoError(err))
	}

	return goja.Undefined()
}

func (e *Engine) templateStringJS(call CallContext) goja.Value {
	inputData := call.Argument(1).Export()

	output, err := e.templator.TemplateString(call.VM, call.Argument(0).String(), inputData)
	if err != nil {
		panic(call.VM.NewGoError(err))
	}

	return call.VM.ToValue(output)
}

//nolint:gomnd
func (e *Engine) templateStringMultipleJS(call CallContext) goja.Value {
	templatePath := call.Argument(0).String()
	inputData := call.Argument(1).Export()
	numTimes := call.Argument(2).ToInteger()

	output, err := e.templator.TemplateStringMultiple(call.VM, templatePath, inputData, int(numTimes))
	if err != nil {
		panic(call.VM.NewGoError(err))
	}

	return call.VM.ToValue(output)
}

func (e *Engine) templateStringInputJS(call CallContext) goja.Value {
	inputData := call.Argument(2).Export() //nolint:gomnd

	output, err := e.templator.TemplateStringInput(call.VM, call.Argument(0).String(), call.Argument(1).String(), inputData, 1)
	if err != nil {
		panic(call.VM.NewGoError(err))
	}

	return call.VM.ToValue(output)
}

//nolint:gomnd
func (e *Engine) templateStringInputMultipleJS(call CallContext) goja.Value {
	name := call.Argument(0).String()
	input := call.Argument(1).String()
	inputData := call.Argument(2).Export()
	numTimes := call.Argument(3).ToInteger()

	output, err := e.templator.TemplateStringInput(call.VM, name, input, inputData, int(numTimes))
	if err != nil {
		panic(call.VM.NewGoError(err))
	}

	return call.VM.ToValue(output)
}
