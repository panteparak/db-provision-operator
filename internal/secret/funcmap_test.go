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
	"bytes"
	"text/template"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("TemplateFuncMap", func() {
	var funcMap template.FuncMap

	BeforeEach(func() {
		funcMap = TemplateFuncMap()
	})

	Describe("urlEncode", func() {
		It("should URL-encode special characters", func() {
			fn := funcMap["urlEncode"].(func(string) string)
			Expect(fn("p@ss!w0rd")).To(Equal("p%40ss%21w0rd"))
		})

		It("should leave alphanumeric characters unchanged", func() {
			fn := funcMap["urlEncode"].(func(string) string)
			Expect(fn("simple")).To(Equal("simple"))
		})

		It("should encode spaces as +", func() {
			fn := funcMap["urlEncode"].(func(string) string)
			Expect(fn("hello world")).To(Equal("hello+world"))
		})
	})

	Describe("urlPathEncode", func() {
		It("should path-encode special characters", func() {
			fn := funcMap["urlPathEncode"].(func(string) string)
			Expect(fn("my/db")).To(Equal("my%2Fdb"))
		})

		It("should encode spaces as %20", func() {
			fn := funcMap["urlPathEncode"].(func(string) string)
			Expect(fn("hello world")).To(Equal("hello%20world"))
		})
	})

	Describe("base64Encode", func() {
		It("should base64-encode a string", func() {
			fn := funcMap["base64Encode"].(func(string) string)
			Expect(fn("hello")).To(Equal("aGVsbG8="))
		})

		It("should handle empty string", func() {
			fn := funcMap["base64Encode"].(func(string) string)
			Expect(fn("")).To(Equal(""))
		})
	})

	Describe("base64Decode", func() {
		It("should base64-decode a valid string", func() {
			fn := funcMap["base64Decode"].(func(string) (string, error))
			result, err := fn("aGVsbG8=")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("hello"))
		})

		It("should return error for invalid base64", func() {
			fn := funcMap["base64Decode"].(func(string) (string, error))
			_, err := fn("not-valid-base64!!!")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("base64Decode"))
		})
	})

	Describe("upper", func() {
		It("should convert to uppercase", func() {
			fn := funcMap["upper"].(func(string) string)
			Expect(fn("hello")).To(Equal("HELLO"))
		})
	})

	Describe("lower", func() {
		It("should convert to lowercase", func() {
			fn := funcMap["lower"].(func(string) string)
			Expect(fn("HELLO")).To(Equal("hello"))
		})
	})

	Describe("title", func() {
		It("should capitalize first letter", func() {
			fn := funcMap["title"].(func(string) string)
			Expect(fn("hello")).To(Equal("Hello"))
		})

		It("should handle empty string", func() {
			fn := funcMap["title"].(func(string) string)
			Expect(fn("")).To(Equal(""))
		})

		It("should handle single character", func() {
			fn := funcMap["title"].(func(string) string)
			Expect(fn("a")).To(Equal("A"))
		})
	})

	Describe("trim", func() {
		It("should trim whitespace", func() {
			fn := funcMap["trim"].(func(string) string)
			Expect(fn("  hello  ")).To(Equal("hello"))
		})
	})

	Describe("trimPrefix", func() {
		It("should trim prefix", func() {
			fn := funcMap["trimPrefix"].(func(string, string) string)
			Expect(fn("hello-world", "hello-")).To(Equal("world"))
		})
	})

	Describe("trimSuffix", func() {
		It("should trim suffix", func() {
			fn := funcMap["trimSuffix"].(func(string, string) string)
			Expect(fn("hello-world", "-world")).To(Equal("hello"))
		})
	})

	Describe("replace", func() {
		It("should replace all occurrences", func() {
			fn := funcMap["replace"].(func(string, string, string) string)
			Expect(fn("hello.world.com", ".", "-")).To(Equal("hello-world-com"))
		})
	})

	Describe("quote", func() {
		It("should wrap in double quotes with escaping", func() {
			fn := funcMap["quote"].(func(string) string)
			Expect(fn(`hello "world"`)).To(Equal(`"hello \"world\""`))
		})

		It("should handle simple string", func() {
			fn := funcMap["quote"].(func(string) string)
			Expect(fn("hello")).To(Equal(`"hello"`))
		})
	})

	Describe("squote", func() {
		It("should wrap in single quotes", func() {
			fn := funcMap["squote"].(func(string) string)
			Expect(fn("hello")).To(Equal("'hello'"))
		})

		It("should escape single quotes", func() {
			fn := funcMap["squote"].(func(string) string)
			Expect(fn("it's")).To(Equal("'it'\\''s'"))
		})
	})

	Describe("default", func() {
		It("should return value when non-empty", func() {
			fn := funcMap["default"].(func(string, string) string)
			Expect(fn("fallback", "actual")).To(Equal("actual"))
		})

		It("should return default when empty", func() {
			fn := funcMap["default"].(func(string, string) string)
			Expect(fn("fallback", "")).To(Equal("fallback"))
		})
	})

	Describe("toJson", func() {
		It("should marshal to JSON", func() {
			fn := funcMap["toJson"].(func(interface{}) (string, error))
			result, err := fn(map[string]string{"key": "value"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(`{"key":"value"}`))
		})
	})

	Describe("join", func() {
		It("should join strings", func() {
			fn := funcMap["join"].(func([]string, string) string)
			Expect(fn([]string{"a", "b", "c"}, ",")).To(Equal("a,b,c"))
		})

		It("should handle empty slice", func() {
			fn := funcMap["join"].(func([]string, string) string)
			Expect(fn([]string{}, ",")).To(Equal(""))
		})
	})

	// ===== Edge Cases (F1-F33) =====

	Describe("urlEncode edge cases", func() {
		It("should encode Unicode characters (F1)", func() {
			fn := funcMap["urlEncode"].(func(string) string)
			result := fn("日本語")
			Expect(result).To(Equal("%E6%97%A5%E6%9C%AC%E8%AA%9E"))
		})

		It("should return empty string for empty input (F20)", func() {
			fn := funcMap["urlEncode"].(func(string) string)
			Expect(fn("")).To(Equal(""))
		})
	})

	Describe("urlPathEncode edge cases", func() {
		It("should encode query-special characters in path segments (F2)", func() {
			fn := funcMap["urlPathEncode"].(func(string) string)
			Expect(fn("my db?q=1")).To(Equal("my%20db%3Fq=1"))
		})
	})

	Describe("base64Encode edge cases", func() {
		It("should encode binary-like content (F3)", func() {
			fn := funcMap["base64Encode"].(func(string) string)
			input := string([]byte{0x00, 0x01, 0xFF, 0xFE})
			result := fn(input)
			Expect(result).To(Equal("AAH//g=="))
		})
	})

	Describe("base64Decode edge cases", func() {
		It("should roundtrip with base64Encode (F4)", func() {
			encode := funcMap["base64Encode"].(func(string) string)
			decode := funcMap["base64Decode"].(func(string) (string, error))
			original := "hello world! 日本語"
			encoded := encode(original)
			decoded, err := decode(encoded)
			Expect(err).NotTo(HaveOccurred())
			Expect(decoded).To(Equal(original))
		})

		It("should return error for invalid base64 '!!!' (F15)", func() {
			fn := funcMap["base64Decode"].(func(string) (string, error))
			_, err := fn("!!!")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("base64Decode"))
		})

		It("should handle truncated base64 missing padding (F16)", func() {
			fn := funcMap["base64Decode"].(func(string) (string, error))
			// "aGVsbG8" is "hello" without the trailing '=' padding
			// StdEncoding requires padding, so this should error
			_, err := fn("aGVsbG8")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("title edge cases", func() {
		It("should capitalize only first letter of multi-word string (F5)", func() {
			fn := funcMap["title"].(func(string) string)
			Expect(fn("hello world")).To(Equal("Hello world"))
		})

		It("should handle empty string (F6)", func() {
			fn := funcMap["title"].(func(string) string)
			Expect(fn("")).To(Equal(""))
		})

		It("should uppercase first rune of multi-byte UTF-8 (F25)", func() {
			fn := funcMap["title"].(func(string) string)
			result := fn("ñoño")
			Expect(result).To(Equal("Ñoño"))
		})

		It("should not panic on invalid UTF-8 bytes (F26)", func() {
			fn := funcMap["title"].(func(string) string)
			input := "\xff\xfe"
			Expect(func() { fn(input) }).NotTo(Panic())
		})
	})

	Describe("default edge cases", func() {
		It("should return value when non-empty (F7)", func() {
			fn := funcMap["default"].(func(string, string) string)
			Expect(fn("fallback", "actual")).To(Equal("actual"))
		})

		It("should return default when value is empty (F8)", func() {
			fn := funcMap["default"].(func(string, string) string)
			Expect(fn("fallback", "")).To(Equal("fallback"))
		})
	})

	Describe("replace edge cases", func() {
		It("should replace all occurrences (F9)", func() {
			fn := funcMap["replace"].(func(string, string, string) string)
			Expect(fn("a.b.c.d", ".", "-")).To(Equal("a-b-c-d"))
		})

		It("should insert between every rune when old string is empty (Go stdlib behavior) (F28)", func() {
			fn := funcMap["replace"].(func(string, string, string) string)
			// strings.ReplaceAll with empty old inserts new before every rune + at end
			result := fn("abc", "", "-")
			Expect(result).To(Equal("-a-b-c-"))
		})
	})

	Describe("quote edge cases", func() {
		It("should handle special chars including backslash and newline (F10)", func() {
			fn := funcMap["quote"].(func(string) string)
			result := fn("he\"llo\\\nworld")
			Expect(result).To(Equal(`"he\"llo\\\nworld"`))
		})

		It("should return two double quotes for empty string (F21)", func() {
			fn := funcMap["quote"].(func(string) string)
			Expect(fn("")).To(Equal(`""`))
		})

		It("should escape control characters (F22)", func() {
			fn := funcMap["quote"].(func(string) string)
			result := fn("\x00\t\n")
			Expect(result).To(Equal(`"\u0000\t\n"`))
		})
	})

	Describe("squote edge cases", func() {
		It("should escape multiple consecutive single quotes (F23)", func() {
			fn := funcMap["squote"].(func(string) string)
			result := fn("it''s")
			Expect(result).To(Equal("'it'\\'''\\''s'"))
		})

		It("should escape string of only quotes (F24)", func() {
			fn := funcMap["squote"].(func(string) string)
			result := fn("'''")
			Expect(result).To(Equal("''\\'''\\'''\\'''"))
		})
	})

	Describe("trimPrefix/trimSuffix edge cases", func() {
		It("should return original when prefix not present (F12)", func() {
			fn := funcMap["trimPrefix"].(func(string, string) string)
			Expect(fn("hello-world", "foo-")).To(Equal("hello-world"))
		})

		It("should return original when suffix not present (F12)", func() {
			fn := funcMap["trimSuffix"].(func(string, string) string)
			Expect(fn("hello-world", "-foo")).To(Equal("hello-world"))
		})

		It("should return original with empty prefix (F29)", func() {
			fn := funcMap["trimPrefix"].(func(string, string) string)
			Expect(fn("hello", "")).To(Equal("hello"))
		})

		It("should return original with empty suffix (F30)", func() {
			fn := funcMap["trimSuffix"].(func(string, string) string)
			Expect(fn("hello", "")).To(Equal("hello"))
		})
	})

	Describe("join edge cases", func() {
		It("should return empty string for empty slice (F13)", func() {
			fn := funcMap["join"].(func([]string, string) string)
			Expect(fn([]string{}, ",")).To(Equal(""))
		})

		It("should handle slice with empty strings (F27)", func() {
			fn := funcMap["join"].(func([]string, string) string)
			Expect(fn([]string{"", "b", ""}, ",")).To(Equal(",b,"))
		})
	})

	Describe("toJson edge cases", func() {
		It("should return 'null' for nil input (F17)", func() {
			fn := funcMap["toJson"].(func(interface{}) (string, error))
			result, err := fn(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("null"))
		})
	})

	Describe("template-level negative cases", func() {
		It("should return parse error for undefined function (F18)", func() {
			_, err := template.New("test").Funcs(TemplateFuncMap()).Parse("{{ noSuchFunc .X }}")
			Expect(err).To(HaveOccurred())
		})

		It("should return execute error for undefined field (F19)", func() {
			tmpl, err := template.New("test").Funcs(TemplateFuncMap()).Parse("{{ .NoSuchField }}")
			Expect(err).NotTo(HaveOccurred())
			var buf bytes.Buffer
			err = tmpl.Execute(&buf, TemplateData{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("can't evaluate field"))
		})

		It("should return error when base64Decode fails in template (F31)", func() {
			tmpl, err := template.New("test").Funcs(TemplateFuncMap()).Parse(`{{ base64Decode "!!!" }}`)
			Expect(err).NotTo(HaveOccurred())
			var buf bytes.Buffer
			err = tmpl.Execute(&buf, TemplateData{})
			Expect(err).To(HaveOccurred())
		})

		It("should iterate N times when ranging over int (Go 1.22+) (F32)", func() {
			// Go 1.22+ allows range over integers: range N iterates 0..N-1
			tmpl, err := template.New("test").Funcs(TemplateFuncMap()).Parse(`{{ range .Port }}x{{ end }}`)
			Expect(err).NotTo(HaveOccurred())
			var buf bytes.Buffer
			err = tmpl.Execute(&buf, TemplateData{Port: 3})
			Expect(err).NotTo(HaveOccurred())
			Expect(buf.String()).To(Equal("xxx"))
		})

		It("should render entire struct without panic (F33)", func() {
			tmpl, err := template.New("test").Funcs(TemplateFuncMap()).Parse(`{{ . }}`)
			Expect(err).NotTo(HaveOccurred())
			var buf bytes.Buffer
			err = tmpl.Execute(&buf, TemplateData{Username: "admin", Port: 5432})
			Expect(err).NotTo(HaveOccurred())
			Expect(buf.String()).To(ContainSubstring("admin"))
		})

		It("should chain functions correctly (F14)", func() {
			tmplStr := `{{ urlEncode (default "fallback" .SSLMode) }}`
			tmpl, err := template.New("test").Funcs(TemplateFuncMap()).Parse(tmplStr)
			Expect(err).NotTo(HaveOccurred())

			var buf bytes.Buffer
			err = tmpl.Execute(&buf, TemplateData{})
			Expect(err).NotTo(HaveOccurred())
			Expect(buf.String()).To(Equal("fallback"))
		})
	})

	Describe("round-trip template rendering", func() {
		It("should render a PostgreSQL connection string with URL encoding", func() {
			tmplStr := `postgresql://{{ urlEncode .Username }}:{{ urlEncode .Password }}@{{ .Host }}:{{ .Port }}/{{ .Database }}?sslmode={{ default "prefer" .SSLMode }}`
			tmpl, err := template.New("test").Funcs(TemplateFuncMap()).Parse(tmplStr)
			Expect(err).NotTo(HaveOccurred())

			data := TemplateData{
				Username: "admin",
				Password: "p@ss!w0rd",
				Host:     "db.example.com",
				Port:     5432,
				Database: "mydb",
			}

			var buf bytes.Buffer
			err = tmpl.Execute(&buf, data)
			Expect(err).NotTo(HaveOccurred())
			Expect(buf.String()).To(Equal("postgresql://admin:p%40ss%21w0rd@db.example.com:5432/mydb?sslmode=prefer"))
		})

		It("should render a template with base64-encoded TLS data", func() {
			tmplStr := `{{ base64Encode .CA }}`
			tmpl, err := template.New("test").Funcs(TemplateFuncMap()).Parse(tmplStr)
			Expect(err).NotTo(HaveOccurred())

			data := TemplateData{
				CA: "-----BEGIN CERTIFICATE-----\nMIIB...\n-----END CERTIFICATE-----",
			}

			var buf bytes.Buffer
			err = tmpl.Execute(&buf, data)
			Expect(err).NotTo(HaveOccurred())
			Expect(buf.String()).To(Equal("LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUIuLi4KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQ=="))
		})

		It("should render a template with default value for empty SSLMode", func() {
			tmplStr := `sslmode={{ default "require" .SSLMode }}`
			tmpl, err := template.New("test").Funcs(TemplateFuncMap()).Parse(tmplStr)
			Expect(err).NotTo(HaveOccurred())

			data := TemplateData{}

			var buf bytes.Buffer
			err = tmpl.Execute(&buf, data)
			Expect(err).NotTo(HaveOccurred())
			Expect(buf.String()).To(Equal("sslmode=require"))
		})

		It("should render a .env style template", func() {
			tmplStr := "DB_HOST={{ .Host }}\nDB_PORT={{ .Port }}\nDB_USER={{ .Username }}"
			tmpl, err := template.New("test").Funcs(TemplateFuncMap()).Parse(tmplStr)
			Expect(err).NotTo(HaveOccurred())

			data := TemplateData{
				Host:     "localhost",
				Port:     5432,
				Username: "admin",
			}

			var buf bytes.Buffer
			err = tmpl.Execute(&buf, data)
			Expect(err).NotTo(HaveOccurred())
			Expect(buf.String()).To(Equal("DB_HOST=localhost\nDB_PORT=5432\nDB_USER=admin"))
		})

		It("should render with replace and upper functions", func() {
			tmplStr := `{{ upper (replace .Host "." "_") }}`
			tmpl, err := template.New("test").Funcs(TemplateFuncMap()).Parse(tmplStr)
			Expect(err).NotTo(HaveOccurred())

			data := TemplateData{Host: "db.example.com"}

			var buf bytes.Buffer
			err = tmpl.Execute(&buf, data)
			Expect(err).NotTo(HaveOccurred())
			Expect(buf.String()).To(Equal("DB_EXAMPLE_COM"))
		})
	})
})
