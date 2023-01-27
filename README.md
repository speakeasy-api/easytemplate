# easytemplate

![181640742-31ab234a-3b39-432e-b899-21037596b360](https://user-images.githubusercontent.com/68016351/196461357-fcb8d90f-cd67-498e-850f-6146c58d0114.png)

**easytemplate** is Go's [text/template](https://pkg.go.dev/text/template) with 🦸 super powers 🦸. It is a templating engine that allows you to use Go's [text/template](https://pkg.go.dev/text/template) syntax, but with the ability to use JavaScript snippets to manipulate data, control templating and run more complex logic while templating.

**easytemplate** powers [Speakeasy's](https://speakeasy-api.dev) SDK Generation product and is used by thousands of developers to generate SDKs for their APIs.

The module includes a number of features on top of the standard [text/template](https://pkg.go.dev/text/template) package, including:

* [x] [Support for JavaScript snippets in templates](#using-javascript).
  * [x] ES5 Support provided by [goja](https://github.com/dop251/goja).
  * [x] Built-in support for [underscore.js](http://underscorejs.org/).
  * [x] Import JavaScripts scripts from other files and inline JavaScript snippets.
  * [x] Modify the templating context from within JavaScript.
* [x] [Controlling the flow of templating within the engine](#controlling-the-flow-of-templating).
* [x] [Inject Go functions into the JavaScript context](#registering-js-functions-from-go), in addition to [Go functions into the templates](#registering-templating-functions).
* [x] [Inject JS functions into the template context.](#registering-js-templating-functions)

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

## Basic Usage

`main.go`

```go
package main

import (
    "fmt"
    "log"
    "os"

    "github.com/speakeasy-api/easytemplate"
)

func main() {
    // Create a new easytemplate engine.
    engine := easytemplate.New()

    // Start the engine from a javascript entrypoint.
    err := engine.RunScript("main.js", data)
    if err != nil {
        log.Fatal(err)
    }
}
```

`main.js`

```js
templateFile("tmpl.stmpl", "out.txt", { name: "John" });
```

`tmpl.stmpl`

```gotemplate
Hello {{ .Local.name }}!

```sjs
console.log("Hello from JavaScript!");
render("This text is rendered from JavaScript!");
sjs```
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

This allows for example:

```gotemplate
{{ templateFile "tmpl.stmpl" "out.txt" .Local }}{{/* Template another file */}}
{{ templateString "tmpl.stmpl" .Local }}{{/* Template another file and include the rendered output in this templates rendered output */}}
```

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

Javascript can be used either via inline snippets or by importing scripts from other files.

If using the `RunScript` method on the engine your entry point will be a JavaScript file where you can setup your environment and start calling template functions.

Alternatively, you can use javascript by embedding snippets within your templates using the `sjs` tag like so:

```gotemplate
```sjs
// JS code here
sjs```
```

The `sjs` snippet can be used anywhere within your template (including multiple snippets) and will be replaced with any "rendered" output returned when using the `render` function.

### Context data

Context data that is available to the templates is also available to JavasScript. Snippets and Files imported with a template file will have access to the same context data as that template file. For example

```gotemplate
```sjs
console.log(context.Global); // The context data provided by the initial call to the templating engine.
console.log(context.Local); // The context data provided to the template file.
sjs```
```

The context object also contains `LocalComputed` and `GlobalComputed` objects that allow you to store computed values that can be later.
`LocalComputed` is only available to the current template file and `GlobalComputed` is available to all templates and scripts, from the point it was set.

### Using the `render` function

The `render` function allows the javascript snippets to render output into the template file before it is templated allowing for dynamic templating. For example:

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
* `render(output)` - Render the output to the template file, if called multiples times the output will be appended to the previous output as a new line. The cumulative output will replace the current `sjs` block in the template file.
  * `output` (string) - The output to render.
* `require(filePath)` - Import a JavaScript file into the global scope.
  * `filePath` (string) - The path to the JavaScript file to import.
* `registerTemplateFunc(name, func)` - Register a template function to be used in the template files.
  * `name` (string) - The name of the function to register.
  * `func` (function) - The function to register.
