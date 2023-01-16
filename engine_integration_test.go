package easytemplate_test

import (
	"os"
	"testing"

	_ "github.com/robertkrimen/otto/underscore"
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
		easytemplate.WithBaseDir("./testdata"),
		easytemplate.WithWriteFunc(func(outFile string, data []byte) error {
			expectedData, ok := expectedFiles[outFile]
			assert.True(t, ok, "unexpected file written: %s", outFile)
			assert.Equal(t, expectedData, string(data))

			delete(expectedFiles, outFile)

			return nil
		}),
	)
	err = e.RunScript("scripts/test.js", map[string]interface{}{
		"Test": "global",
	})
	assert.NoError(t, err)

	assert.Empty(t, expectedFiles, "not all expected files were written")
}
