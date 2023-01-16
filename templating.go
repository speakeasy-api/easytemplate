package easytemplate

import "github.com/robertkrimen/otto"

//nolint:gomnd
func (e *Engine) templateFileJS(call otto.FunctionCall) otto.Value {
	inputData, err := call.Argument(2).Export()
	if err != nil {
		panic(call.Otto.MakeCustomError("templateFile", err.Error()))
	}

	if err := e.templator.TemplateFile(call.Otto, call.Argument(0).String(), call.Argument(1).String(), inputData); err != nil {
		panic(call.Otto.MakeCustomError("templateFile", err.Error()))
	}

	return otto.Value{}
}

func (e *Engine) templateStringJS(call otto.FunctionCall) otto.Value {
	inputData, err := call.Argument(1).Export()
	if err != nil {
		panic(call.Otto.MakeCustomError("templateString", err.Error()))
	}

	output, err := e.templator.TemplateString(call.Otto, call.Argument(0).String(), inputData)
	if err != nil {
		panic(call.Otto.MakeCustomError("templateString", err.Error()))
	}

	val, err := otto.ToValue(output)
	if err != nil {
		panic(call.Otto.MakeCustomError("templateString", err.Error()))
	}

	return val
}
