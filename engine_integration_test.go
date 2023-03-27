package easytemplate_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/dop251/goja"
	"github.com/speakeasy-api/easytemplate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEngine_RunScript_Success(t *testing.T) {
	files, err := os.ReadDir("./testdata/expected")
	require.NoError(t, err)

	expectedFiles := make(map[string]string, len(files))

	for _, file := range files {
		data, err := os.ReadFile("./testdata/expected/" + file.Name())
		require.NoError(t, err)

		expectedFiles[file.Name()] = string(data)
	}

	e := easytemplate.New(
		easytemplate.WithSearchLocations([]string{"./testdata"}),
		easytemplate.WithWriteFunc(func(outFile string, data []byte) error {
			expectedData, ok := expectedFiles[outFile]
			if ok {
				assert.Equal(t, expectedData, string(data))
				delete(expectedFiles, outFile)
			} else {
				os.WriteFile("./testdata/expected/"+outFile, data, 0644)
			}

			return nil
		}),
		easytemplate.WithJSFuncs(map[string]func(call easytemplate.CallContext) goja.Value{
			"multiply": func(call easytemplate.CallContext) goja.Value {
				a := call.Argument(0).ToInteger()
				b := call.Argument(1).ToInteger()

				return call.VM.ToValue(a * b)
			},
		}),
		easytemplate.WithTemplateFuncs(map[string]any{
			"toFloatWithPrecision": func(i int64, precision int) string {
				return fmt.Sprintf("%.*f", precision, float64(i))
			},
		}),
	)
	err = e.RunScript("scripts/test.js", map[string]interface{}{
		"Test": "global",
	})
	assert.NoError(t, err)

	assert.Empty(t, expectedFiles, "not all expected files were written")
}
