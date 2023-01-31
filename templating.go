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

func (e *Engine) templateStringJS(call CallContext) goja.Value {
	inputData := call.Argument(1).Export()

	output, err := e.templator.TemplateString(call.VM, call.Argument(0).String(), inputData)
	if err != nil {
		panic(call.VM.NewGoError(err))
	}

	return call.VM.ToValue(output)
}

func (e *Engine) templateStringInputJS(call CallContext) goja.Value {
	inputData := call.Argument(2).Export() //nolint:gomnd

	output, err := e.templator.TemplateStringInput(call.VM, call.Argument(0).String(), call.Argument(1).String(), inputData)
	if err != nil {
		panic(call.VM.NewGoError(err))
	}

	return call.VM.ToValue(output)
}
