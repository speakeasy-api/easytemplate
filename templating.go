package easytemplate

import (
	"github.com/dop251/goja"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func (e *Engine) templateFileJS(call CallContext) goja.Value {
	templateFile := call.Argument(0).String()
	outFile := call.Argument(1).String()
	inputData := call.Argument(2).Export() //nolint:gomnd

	ctx := call.Ctx
	_, span := e.tracer.Start(ctx, "js:templateFile", trace.WithAttributes(
		attribute.String("templateFile", templateFile),
		attribute.String("outFile", outFile),
	))
	defer span.End()

	if err := e.templator.TemplateFile(call.Ctx, call.VM, templateFile, outFile, inputData); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()

		panic(call.VM.NewGoError(err))
	}

	return goja.Undefined()
}

func (e *Engine) templateStringJS(call CallContext) goja.Value {
	templateFile := call.Argument(0).String()
	inputData := call.Argument(1).Export()

	ctx := call.Ctx
	_, span := e.tracer.Start(ctx, "js:templateString", trace.WithAttributes(
		attribute.String("templateFile", templateFile),
	))
	defer span.End()

	output, err := e.templator.TemplateString(call.Ctx, call.VM, templateFile, inputData)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()

		panic(call.VM.NewGoError(err))
	}

	return call.VM.ToValue(output)
}

func (e *Engine) templateStringInputJS(call CallContext) goja.Value {
	name := call.Argument(0).String()
	input := call.Argument(1).String()
	inputData := call.Argument(2).Export() //nolint:gomnd

	ctx := call.Ctx
	_, span := e.tracer.Start(ctx, "js:templateStringInput", trace.WithAttributes(
		attribute.String("name", name),
	))
	defer span.End()

	output, err := e.templator.TemplateStringInput(call.Ctx, call.VM, name, input, inputData)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()

		panic(call.VM.NewGoError(err))
	}

	return call.VM.ToValue(output)
}

func (e *Engine) recurseJS(call CallContext) goja.Value {
	output, err := e.templator.Recurse(call.VM, int(call.Argument(0).ToInteger()))
	if err != nil {
		panic(call.VM.NewGoError(err))
	}

	return call.VM.ToValue(output)
}
