// Package utils contains utility functions.
package utils

import "regexp"

// ReplaceAllStringSubmatchFunc replaces all submatches with the result of the repl function.
func ReplaceAllStringSubmatchFunc(re *regexp.Regexp, str string, repl func([]string) (string, error)) (string, error) {
	result := ""
	lastIndex := 0

	for _, v := range re.FindAllSubmatchIndex([]byte(str), -1) {
		groups := []string{}
		for i := 0; i < len(v); i += 2 {
			if v[i] == -1 || v[i+1] == -1 {
				groups = append(groups, "")
			} else {
				groups = append(groups, str[v[i]:v[i+1]])
			}
		}

		replStr, err := repl(groups)
		if err != nil {
			return "", err
		}

		result += str[lastIndex:v[0]] + replStr
		lastIndex = v[1]
	}

	return result + str[lastIndex:], nil
}
