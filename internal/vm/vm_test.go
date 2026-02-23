package vm_test

import (
	"context"
	"testing"

	"github.com/speakeasy-api/easytemplate/internal/vm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVM_Run_Runtime_Success(t *testing.T) {
	v, err := vm.New(nil)
	require.NoError(t, err)

	typeScript := `console.log("hello world");`

	_, err = v.Run(context.Background(), "test", typeScript)
	assert.NoError(t, err)
}

func TestVM_Run_Runtime_WithRandSource_Success(t *testing.T) {
	v, err := vm.New(func() float64 {
		return 0
	})
	require.NoError(t, err)

	typeScript := `console.log("hello world");`

	_, err = v.Run(context.Background(), "test", typeScript)
	assert.NoError(t, err)
}

func TestVM_Run_Runtime_Errors(t *testing.T) {
	v, err := vm.New(nil)
	require.NoError(t, err)

	typeScript := `type Test = {
  Name: string;
};
function test(input: Test): string {
	throw new Error("test error");
}

test({ Name: "test" });`

	_, err = v.Run(context.Background(), "test", typeScript)
	assert.Equal(t, "failed to run script Error: test error\n\tat test (test:5:7(3))\n\tat test:8:5(6)\n: script runtime failure", err.Error())
}
