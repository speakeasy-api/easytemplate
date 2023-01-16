package utils_test

import (
	"regexp"
	"testing"

	"github.com/speakeasy-api/easytemplate/internal/utils"
	"github.com/stretchr/testify/assert"
)

func TestReplaceAllStringSubmatchFunc_Success(t *testing.T) {
	type args struct {
		re   *regexp.Regexp
		str  string
		repl func([]string) (string, error)
	}
	tests := []struct {
		name    string
		args    args
		wantOut string
	}{
		{
			name: "success",
			args: args{
				re:  regexp.MustCompile("(?ms)(^```sjs\\s+(.*?)^sjs```)"),
				str: "{{ .Global.Test }}\n```sjs\nrender(\"something\")\nsjs```\n{{ .Local.Test }}\n```sjs\nrender(\"something else\")\nsjs```\n{{ .Global.Test }}\n",
				repl: func(groups []string) (string, error) {
					return "test", nil
				},
			},
			wantOut: "{{ .Global.Test }}\ntest\n{{ .Local.Test }}\ntest\n{{ .Global.Test }}\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := utils.ReplaceAllStringSubmatchFunc(tt.args.re, tt.args.str, tt.args.repl)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantOut, out)
		})
	}
}
