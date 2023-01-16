//go:generate mockgen -destination=./mocks/template_mock.go -package mocks github.com/speakeasy-api/easytemplate/internal/template VM
package template_test

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/robertkrimen/otto"
	"github.com/speakeasy-api/easytemplate/internal/template"
	"github.com/speakeasy-api/easytemplate/internal/template/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplator_TemplateFile_Success(t *testing.T) {
	type fields struct {
		contextData interface{}
		template    string
	}
	type args struct {
		templatePath string
		outFile      string
		inputData    any
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantOut string
	}{
		{
			name: "success",
			fields: fields{
				contextData: map[string]interface{}{"Test": "global"},
				template:    "{{ .Global.Test }}\n{{ .Local.Test }}",
			},
			args: args{
				templatePath: "test",
				inputData:    map[string]interface{}{"Test": "local"},
			},
			wantOut: "global\nlocal",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			vm := mocks.NewMockVM(ctrl)

			context := &template.Context{
				Global: tt.fields.contextData,
				Local:  tt.args.inputData,
			}
			o := otto.New()
			contextVal, err := o.ToValue(context)
			require.NoError(t, err)

			vm.EXPECT().Get("context").Return(otto.Value{}, nil).Times(1)
			vm.EXPECT().Set("context", context).Return(nil).Times(1)
			vm.EXPECT().Get("context").Return(contextVal, nil).Times(1)
			vm.EXPECT().Set("context", otto.Value{}).Return(nil).Times(1)

			tr := &template.Templator{
				ReadFunc: func(s string) ([]byte, error) {
					assert.Equal(t, tt.args.templatePath, s)
					return []byte(tt.fields.template), nil
				},
				WriteFunc: func(s string, b []byte) error {
					assert.Equal(t, tt.args.outFile, s)
					assert.Equal(t, tt.wantOut, string(b))
					return nil
				},
				ContextData: tt.fields.contextData,
			}
			err = tr.TemplateFile(vm, tt.args.templatePath, tt.args.outFile, tt.args.inputData)
			assert.NoError(t, err)
		})
	}
}

func TestTemplator_TemplateString_Success(t *testing.T) {
	type fields struct {
		contextData interface{}
		template    string
		tmplFuncs   map[string]any
		includedJS  string
	}
	type args struct {
		templatePath string
		inputData    any
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantOut string
	}{
		{
			name: "successfully templates simple template",
			fields: fields{
				contextData: map[string]interface{}{"Test": "global"},
				template:    "{{ .Global.Test }}\n{{ .Local.Test }}",
			},
			args: args{
				templatePath: "test",
				inputData:    map[string]interface{}{"Test": "local"},
			},
			wantOut: "global\nlocal",
		},
		{
			name: "successfully templates template using injected template function",
			fields: fields{
				contextData: map[string]interface{}{"Test": "global"},
				template:    "{{ testFunc .Global.Test }}",
				tmplFuncs: map[string]any{
					"testFunc": func(s string) string {
						return s + " handled"
					},
				},
			},
			args: args{
				templatePath: "test",
			},
			wantOut: "global handled",
		},
		{
			name: "successfully templates template with sjs block",
			fields: fields{
				contextData: map[string]interface{}{"Test": "global"},
				template:    "{{ .Global.Test }}\n```sjs\nconsole.log(\"test\");\nsjs```",
				includedJS:  "console.log(\"test\");\n",
			},
			args: args{
				templatePath: "test",
			},
			wantOut: "global\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			vm := mocks.NewMockVM(ctrl)

			context := &template.Context{
				Global: tt.fields.contextData,
				Local:  tt.args.inputData,
			}
			o := otto.New()
			contextVal, err := o.ToValue(context)
			require.NoError(t, err)

			vm.EXPECT().Get("context").Return(otto.Value{}, nil).Times(1)
			vm.EXPECT().Set("context", context).Return(nil).Times(1)

			if tt.fields.includedJS != "" {
				vm.EXPECT().Compile("", tt.fields.includedJS).Return(&otto.Script{}, nil).Times(1)

				vm.EXPECT().Get("render").Return(otto.Value{}, nil).Times(1)
				vm.EXPECT().Set("render", gomock.Any()).Return(nil).Times(1)
				vm.EXPECT().Run(&otto.Script{}).Return(otto.Value{}, nil).Times(1)
				vm.EXPECT().Set("render", otto.Value{}).Return(nil).Times(1)
			}

			vm.EXPECT().Get("context").Return(contextVal, nil).Times(1)
			vm.EXPECT().Set("context", otto.Value{}).Return(nil).Times(1)

			tr := &template.Templator{
				ReadFunc: func(s string) ([]byte, error) {
					assert.Equal(t, tt.args.templatePath, s)
					return []byte(tt.fields.template), nil
				},
				ContextData: tt.fields.contextData,
				TmplFuncs:   tt.fields.tmplFuncs,
			}
			out, err := tr.TemplateString(vm, tt.args.templatePath, tt.args.inputData)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantOut, out)
		})
	}
}
