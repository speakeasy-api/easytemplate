module github.com/speakeasy-api/easytemplate

go 1.20

require (
	github.com/dop251/goja v0.0.0
	github.com/dop251/goja/debugger v0.0.0
	github.com/dop251/goja_nodejs v0.0.0-20240728170619-29b559befffc
	github.com/evanw/esbuild v0.23.1
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible
	github.com/golang/mock v1.6.0
	github.com/stretchr/testify v1.8.4
	go.opentelemetry.io/otel v1.24.0
	go.opentelemetry.io/otel/trace v1.24.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dlclark/regexp2 v1.11.4 // indirect
	github.com/google/go-dap v0.12.0 // indirect
	github.com/google/pprof v0.0.0-20240727154555-813a5fbdbec8 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/sys v0.16.0 // indirect
	golang.org/x/text v0.16.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/dop251/goja => github.com/speakeasy-api/goja v0.0.0-20260223084236-ed0328a0a462

replace github.com/dop251/goja/debugger => github.com/speakeasy-api/goja/debugger v0.0.0-20260223084236-ed0328a0a462
