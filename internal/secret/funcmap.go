/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package secret

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"text/template"
	"unicode"
	"unicode/utf8"
)

// TemplateFuncMap returns the custom template functions available in SecretTemplate.Data.
// All functions use stdlib only — no external dependencies.
func TemplateFuncMap() template.FuncMap {
	return template.FuncMap{
		// URL encoding
		"urlEncode":     url.QueryEscape,
		"urlPathEncode": url.PathEscape,

		// Base64
		"base64Encode": func(s string) string {
			return base64.StdEncoding.EncodeToString([]byte(s))
		},
		"base64Decode": func(s string) (string, error) {
			b, err := base64.StdEncoding.DecodeString(s)
			if err != nil {
				return "", fmt.Errorf("base64Decode: %w", err)
			}
			return string(b), nil
		},

		// Case transforms
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"title": titleCase,

		// Trimming
		"trim":       strings.TrimSpace,
		"trimPrefix": strings.TrimPrefix,
		"trimSuffix": strings.TrimSuffix,

		// String manipulation
		"replace": func(s, old, new string) string {
			return strings.ReplaceAll(s, old, new)
		},

		// Quoting
		"quote": func(s string) string {
			b, _ := json.Marshal(s) // json.Marshal produces a valid double-quoted string with escaping
			return string(b)
		},
		"squote": func(s string) string {
			return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
		},

		// Default value
		"default": func(def, val string) string {
			if val == "" {
				return def
			}
			return val
		},

		// JSON
		"toJson": func(v interface{}) (string, error) {
			b, err := json.Marshal(v)
			if err != nil {
				return "", fmt.Errorf("toJson: %w", err)
			}
			return string(b), nil
		},

		// Join
		"join": func(elems []string, sep string) string {
			return strings.Join(elems, sep)
		},
	}
}

// titleCase capitalizes the first letter of a string, leaving the rest unchanged.
func titleCase(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}
