# easytemplate

[![made-with-Go](https://img.shields.io/badge/Made%20with-Go-1f425f.svg)](https://go.dev/)
[![Go Report Card](https://goreportcard.com/badge/github.com/speakeasy-api/easytemplate)](https://goreportcard.com/report/github.com/speakeasy-api/easytemplate)
[![GoDoc](https://godoc.org/github.com/speakeasy-api/easytemplate?status.svg)](https://godoc.org/github.com/speakeasy-api/easytemplate)

**easytemplate** is Go's [text/template](https://pkg.go.dev/text/template) with 🦸 super powers 🦸. It is a templating engine that allows you to use Go's [text/template](https://pkg.go.dev/text/template) syntax, but with the ability to use JavaScript snippets to manipulate data, control templating and run more complex logic while templating.

**easytemplate** powers [Speakeasy's](https://speakeasy-api.dev) SDK Generation product, which is used by thousands of developers to generate SDKs for their APIs.

The module includes a number of features on top of the standard [text/template](https://pkg.go.dev/text/template) package, including:

* [x] [Support for JavaScript snippets in templates](#using-javascript).
  * [x] ES5 Support provided by [goja](https://github.com/dop251/goja).
  * [x] Built-in support for [underscore.js](http://underscorejs.org/).
  * [x] Import JavaScripts scripts from other files and inline JavaScript snippets.
  * [x] Modify the templating context from within JavaScript.
* [x] [Controlling the flow of templating within the engine](#controlling-the-flow-of-templating).
* [x] [Inject Go functions into the JavaScript context](#registering-js-functions-from-go), in addition to [Go functions into the templates](#registering-templating-functions).
* [x] [Inject JS functions into the template context.](#registering-js-templating-functions)
* [x] [Interactive debugging](#debugging) of JS/TS code in VS Code via the [goja DAP debugger](https://github.com/speakeasy-api/goja/tree/feat/debugger/debugger).

## Debugging

easytemplate supports interactive debugging of JavaScript and TypeScript code via the [goja DAP debugger](https://github.com/speakeasy-api/goja/tree/feat/debugger/debugger). This lets you set breakpoints, step through code, and inspect variables in VS Code — both in the initial script execution phase and during template rendering when JS/TS helper functions are invoked.

To enable debugging, pass a debug port when creating the engine:

```go
engine := easytemplate.New(
    easytemplate.WithDebugger(4711),
)
```

When a debug port is set, the engine will:

1. Compile all scripts with debug metadata (source maps, variable names)
2. Start a DAP server on the specified TCP port
3. Wait for a VS Code debugger to connect before executing scripts

You can then set breakpoints in your `.js`/`.ts` files and step through both the initial `RunScript` phase and any `registerTemplateFunc` functions that execute during template rendering.

See the [goja debugger README](https://github.com/speakeasy-api/goja/tree/feat/debugger/debugger) for VS Code extension installation, launch configuration, and full feature documentation.

## Installation

```bash
go get github.com/speakeasy-api/easytemplate
```

## How does it work?

Using [goja](https://github.com/dop251/goja), `easytemplate` adds a superset of functionality to Go's [text/template](https://pkg.go.dev/text/template) package, with minimal dependencies and no bulky external JS runtime.

`easytemplate` allows you to control templating directly from scripts or other templates which among other things, allows you to:

* Break templates into smaller, more manageable templates and reuse them, while also including them within one another without the need for your Go code to know about them or control the flow of templating them.
* Provide templates and scripts at runtime allowing pluggable templating for your application.
* Separate your templates and scripts from your Go code, allowing you to easily update them without having to recompile your application and keeping concerns separate.

## Documentation

See the [documentation](https://pkg.go.dev/github.com/speakeasy-api/easytemplate) for more information and instructions on how to use the package.

## Basic Usage Example

`main.go`

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/speakeasy-api/easytemplate"
)

func main() {
    // Create and initialize a new easytemplate engine.
    engine := easytemplate.New()
    ctx := context.Background()
    data := 0
    engine.Init(ctx, data)
    // Start the engine from a javascript entrypoint.
    err := engine.RunScript(ctx, "main.js")
    if err != nil {
        log.Fatal(err)
    }
}
```

`main.js`

```js
// From our main entrypoint, we can render a template file, the last argument is the data to pass to the template.
templateFile("tmpl.stmpl", "out.txt", { name: "John" });
```

`tmpl.stmpl`

In the below template we are using the `name` variable from the data we passed to the template from main.js.

We then also have an embedded JavaScript block that both renders output (the sjs block is replaced in the final output by any rendered text or just removed if nothing is rendered) and sets up additional data available to the template that it then uses to render another template inline.

```go
Hello {{ .Local.name }}!

```sjs
console.log("Hello from JavaScript!"); // Logs message to stdout useful for debugging.

render("This text is rendered from JavaScript!"); 

context.LocalComputed.SomeComputedText = "This text is computed from JavaScript!";
sjs```

{{ templateString "tmpl2.stmpl" .LocalComputed }}
```

`tmpl2.stmpl`

```go
And then we are showing some computed text from JavaScript:
{{ .Local.SomeComputedText }}
```

The rendered file `out.txt`

```text
Hello John!

This text is rendered from JavaScript!

And then we are showing some computed text from JavaScript:
This text is computed from JavaScript!
```

## Templating

As the templating is based on Go's [text/template](https://pkg.go.dev/text/template) package, the syntax is exactly the same and can be used mostly as a drop in replacement apart from the differing API to start templating.

Where the engine differs is in the ability to control the flow of templating from within the templates and scripts themselves. This means from a single entry point you can start multiple templates and scripts, and even start templates and scripts from within templates and scripts.

### Starting the engine

A number of methods are available to start the engine, including:

* `RunScript(scriptFilePath string, data any) error` - Start the engine from a JavaScript file.
  * `scriptFilePath` (string) - The path to the JavaScript file to start the engine from.
  * `data` (any) - Context data to provide to templates and scripts. Available as `{{.Global}}` in templates and `context.Global` in scripts.
* `RunTemplate(templateFile string, outFile string, data any) error` - Start the engine from a template file and write the output to the template to a file.
  * `templateFile` (string) - The path to the template file to start the engine from.
  * `outFile` (string) - The path to the output file to write the rendered template to.
  * `data` (any) - Context data to provide to templates and scripts. Available as `{{.Global}}` in templates and `context.Global` in scripts.
* `RunTemplateString(templateFile string, data any) (string, error)` - Start the engine from a template file and return the rendered template as a string.
  * `templateFile` (string) - The path to the template file to start the engine from.
  * `data` (any) - Context data to provide to templates and scripts. Available as `{{.Global}}` in templates and `context.Global` in scripts.

### Controlling the flow of templating

The engine allows you to control the flow of templating from within templates and scripts themselves. This means from a single entry point you can start multiple templates and scripts.

This is done by calling the following functions from within templates and scripts:

* `templateFile(templateFile string, outFile string, data any) error` - Start a template file and write the output to the template to a file.
  * `templateFile` (string) - The path to the template file to start the engine from.
  * `outFile` (string) - The path to the output file to write the rendered template to.
  * `data` (any) - Context data to provide to templates and scripts. Available as `{{.Local}}` in templates and `context.Local` in scripts.
* `templateString(templateFile string, data any) (string, error)` - Start a template file and return the rendered template as a string.
  * `templateFile` (string) - The path to the template file to start the engine from.
  * `data` (any) - Context data to provide to templates and scripts. Available as `{{.Local}}` in templates and `context.Local` in scripts.
* `templateStringInput(templateName string, templateString string, data any) (string, error)` - Template the input string and return the rendered template as a string.
  * `templateName` (string) - The name of the template to render.
  * `templateString` (string) - An input template string to template.
  * `data` (any) - Context data to provide to templates and scripts. Available as `{{.Local}}` in templates and `context.Local` in scripts.
* `recurse(recursions int) string` - Recurse the current template file, recursions is the number of times to recurse the template file.
  * `recursions` (int) - The number of times to recurse the template file.

This allows for example:

```gotemplate
{{ templateFile "tmpl.stmpl" "out.txt" .Local }}{{/* Template another file */}}
{{ templateString "tmpl.stmpl" .Local }}{{/* Template another file and include the rendered output in this templates rendered output */}}
{{ templateStringInput "Hello {{ .Local.name }}" .Local }}{{/* Template a string and include the rendered output in this templates rendered output */}}
```

#### Recursive templating

It is possible with the `recurse` function in a template to render the same template multiple times. This can be useful when data to render parts of the template are only available after you have rendered it at least once.

For example:

```go
{{- recurse 1 -}}
{{"{{.RecursiveComputed.Names}}"}}{{/* Render the names of the customers after we have iterated over them later */}}
{{range .Local.Customers}}
{{- addName .RecursiveComputed.Names (print .FirstName " " .LastName) -}}
{{.FirstName}} {{.LastName}}
{{end}}
```

Note: The `recurse` function must be called as the first thing in the template on its own line.

### Registering templating functions

The engine allows you to register custom templating functions from Go which can be used within the templates.

```go
engine := easytemplate.New(
  easytemplate.WithTemplateFuncs(map[string]any{
    "hello": func(name string) string {
      return fmt.Sprintf("Hello %s!", name)
    },
  }),
)
```

## Using JavaScript

JavaScript can be used either via inline snippets or by importing scripts from other files.

If using the `RunScript` method on the engine your entry point will be a JavaScript file where you can setup your environment and start calling template functions.

Alternatively, you can use JavaScript by embedding snippets within your templates using the `sjs` tag like so:

```gotemplate
```sjs
// JS code here
sjs```
```

The `sjs` snippet can be used anywhere within your template (including multiple snippets) and will be replaced with any "rendered" output returned when using the `render` function.

Naive transformation of typescript code is supported through [esbuild](https://esbuild.github.io/api/#transformation). This means that you can directly import typescript code and use type annotations in place of any JavaScript. However, be aware:

* EasyTemplate will not perform type checking itself. Type annotations are transformed into commented out code.  
* Scripts/Snippets are not bundled, but executed as a single module on the global scope. This means no `import` statements are possible. [Instead, the global `require` function](#importing-javascript) is available to directly execute JS/TS code.

### Context data

Context data that is available to the templates is also available to JavaScript. Snippets and Files imported with a template file will have access to the same context data as that template file. For example

```gotemplate
```sjs
console.log(context.Global); // The context data provided by the initial call to the templating engine.
console.log(context.Local); // The context data provided to the template file.
sjs```
```

The context object also contains `LocalComputed` and `GlobalComputed` objects that allow you to store computed values that can be later.
`LocalComputed` is only available to the current template file and `GlobalComputed` is available to all templates and scripts, from the point it was set.

### Using the `render` function

The `render` function allows the JavaScript snippets to render output into the template file before it is templated allowing for dynamic templating. For example:

```gotemplate
{{ .Local.firstName }} ```sjs
if (context.Local.lastName) {
  render("{{ .Local.lastName }}"); // Only add the last name if it is provided.
}

render("Hello from JavaScript!");
sjs```
```

The above example will render the following output, replacing the `sjs` block in the template before rendering:

```gotemplate
{{ .Local.firstName }} {{ .Local.lastName }}
Hello from JavaScript!
```

The output from the `render` function can just be plain text or more templating instructions as shown above.

### Registering JS templating functions

In addition from register functions from Go to be used in the templates, you can also register functions from JavaScript. For example:

```javascript
registerTemplateFunc("hello", function(name) {
  return "Hello " + name + "!";
});

// or
function hello(name) {
  return "Hello " + name + "!";
}

registerTemplateFunc("hello", hello);
```

The above functions can then be used in the templates like so:

```gotemplate
{{ hello "John" }}
```

Any functions registered will then be available for templates that are templated after the function is registered.

### Registering JS functions from Go

You can also register JavaScript functions from Go. For example:

```go
engine := easytemplate.New(
  easytemplate.WithJSFuncs(map[string]func(call easytemplate.CallContext) goja.Value{
    "hello": func(call easytemplate.CallContext) goja.Value {
      name := call.Argument(0).String()
      return call.VM.ToValue("Hello " + name + "!")
    }
  }),
)
```

### Importing JavaScript

JavaScript both inline and in files can import other JavaScript files using the built in `require` function (*Note*: this `require` doesn't work like Node's `require` and is only used to import other JavaScript files into the global scope):

```js
require("path/to/file.js");
```

Any functions or variables defined in the imported file will be available in the global scope.

### Using Underscore.js

The [underscore.js](http://underscorejs.org/) library is included by default and can be used in your JavaScript snippets/code.

```js
_.each([1, 2, 3], console.log);
```

### Importing External Javascript Libraries

Using `WithJSFiles` you can import external JavaScript libraries into the global scope. For example:

```go
engine := easytemplate.New(
  easytemplate.WithJSFiles("faker.min.js", "<CONTENT OF FILE HERE>"),
)
```

The imported code will be available in the global scope.

### Available Engine functions to JS

The following functions are available to JavaScript from the templating engine:

* `templateFile(templateFilePath, outFilePath, data)` - Render a template file to the specified output file path.
  * `templateFilePath` (string) - The path to the template file to render.
  * `outFilePath` (string) - The path to the output file to render to.
  * `data` (object) - Data available to the template as `Local` context ie `{name: "John"}` is available as `{{ .Local.name }}`.
* `templateString(templateString, data)` - Render a template and return the rendered output.
  * `templateString` (string) - The template string to render.
  * `data` (object) - Data available to the template as `Local` context ie `{name: "John"}` is available as `{{ .Local.name }}`.
* `templateStringInput(templateName, templateString, data)` - Render a template and return the rendered output.
  * `templateName` (string) - The name of the template to render.
  * `templateString` (string) - The template string to render.
  * `data` (object) - Data available to the template as `Local` context ie `{name: "John"}` is available as `{{ .Local.name }}`.
* `render(output)` - Render the output to the template file, if called multiples times the output will be appended to the previous output as a new line. The cumulative output will replace the current `sjs` block in the template file.
  * `output` (string) - The output to render.
* `require(filePath)` - Import a JavaScript file into the global scope.
  * `filePath` (string) - The path to the JavaScript file to import.
* `registerTemplateFunc(name, func)` - Register a template function to be used in the template files.
  * `name` (string) - The name of the function to register.
  * `func` (function) - The function to register.
