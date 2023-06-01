package vm_test

import (
	"testing"

	"github.com/speakeasy-api/easytemplate/internal/vm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVM_Run_Runtime_Errors(t *testing.T) {
	v, err := vm.New()
	require.NoError(t, err)

	typeScript := `type Test = {
  Name: string;
};
function test(input: Test): string {
	throw new Error("test error");
}

test({ Name: "test" });`

	_, err = v.Run("test", typeScript)
	assert.Equal(t, "failed to run script Error: test error\n\tat test (test:2:9(3))\n\tat test:8:5:*(6)\n: script runtime failure", err.Error())
}
