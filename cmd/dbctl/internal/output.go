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

package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"sigs.k8s.io/yaml"
)

// OutputFormat represents the output format type
type OutputFormat string

const (
	FormatTable OutputFormat = "table"
	FormatYAML  OutputFormat = "yaml"
	FormatJSON  OutputFormat = "json"
)

// ParseOutputFormat parses a string into an OutputFormat
func ParseOutputFormat(s string) OutputFormat {
	switch strings.ToLower(s) {
	case "yaml", "yml":
		return FormatYAML
	case "json":
		return FormatJSON
	default:
		return FormatTable
	}
}

// Printer handles output formatting
type Printer struct {
	format OutputFormat
	writer io.Writer
}

// NewPrinter creates a new Printer
func NewPrinter(format OutputFormat, writer io.Writer) *Printer {
	return &Printer{
		format: format,
		writer: writer,
	}
}

// PrintResult prints a result message
func (p *Printer) PrintResult(message string) {
	fmt.Fprintln(p.writer, message)
}

// PrintTable prints data in table format
func (p *Printer) PrintTable(headers []string, rows [][]string) error {
	if p.format != FormatTable {
		// For non-table formats, convert to list of maps
		var data []map[string]string
		for _, row := range rows {
			item := make(map[string]string)
			for i, header := range headers {
				if i < len(row) {
					item[header] = row[i]
				}
			}
			data = append(data, item)
		}
		return p.PrintData(data)
	}

	w := tabwriter.NewWriter(p.writer, 0, 0, 2, ' ', 0)

	// Print headers
	fmt.Fprintln(w, strings.Join(headers, "\t"))

	// Print rows
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}

	return w.Flush()
}

// PrintData prints data in the configured format
func (p *Printer) PrintData(data interface{}) error {
	switch p.format {
	case FormatYAML:
		return p.printYAML(data)
	case FormatJSON:
		return p.printJSON(data)
	default:
		// For table format with arbitrary data, use YAML
		return p.printYAML(data)
	}
}

// printYAML prints data as YAML
func (p *Printer) printYAML(data interface{}) error {
	output, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}
	fmt.Fprint(p.writer, string(output))
	return nil
}

// printJSON prints data as JSON
func (p *Printer) printJSON(data interface{}) error {
	output, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	fmt.Fprintln(p.writer, string(output))
	return nil
}

// DatabaseOutput represents database info for output
type DatabaseOutput struct {
	Name       string `json:"name" yaml:"name"`
	Engine     string `json:"engine" yaml:"engine"`
	Size       string `json:"size,omitempty" yaml:"size,omitempty"`
	Owner      string `json:"owner,omitempty" yaml:"owner,omitempty"`
	Encoding   string `json:"encoding,omitempty" yaml:"encoding,omitempty"`
	Collation  string `json:"collation,omitempty" yaml:"collation,omitempty"`
	Extensions string `json:"extensions,omitempty" yaml:"extensions,omitempty"`
}

// UserOutput represents user info for output
type UserOutput struct {
	Username   string `json:"username" yaml:"username"`
	Roles      string `json:"roles,omitempty" yaml:"roles,omitempty"`
	Attributes string `json:"attributes,omitempty" yaml:"attributes,omitempty"`
	ValidUntil string `json:"validUntil,omitempty" yaml:"validUntil,omitempty"`
}

// RoleOutput represents role info for output
type RoleOutput struct {
	RoleName   string `json:"roleName" yaml:"roleName"`
	Attributes string `json:"attributes,omitempty" yaml:"attributes,omitempty"`
	Members    string `json:"members,omitempty" yaml:"members,omitempty"`
}

// GrantOutput represents grant info for output
type GrantOutput struct {
	Grantee    string `json:"grantee" yaml:"grantee"`
	ObjectType string `json:"objectType,omitempty" yaml:"objectType,omitempty"`
	ObjectName string `json:"objectName,omitempty" yaml:"objectName,omitempty"`
	Privileges string `json:"privileges" yaml:"privileges"`
}

// HealthOutput represents health check info for output
type HealthOutput struct {
	Healthy bool   `json:"healthy" yaml:"healthy"`
	Engine  string `json:"engine" yaml:"engine"`
	Version string `json:"version,omitempty" yaml:"version,omitempty"`
	Latency string `json:"latency,omitempty" yaml:"latency,omitempty"`
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
}

// FormatSize formats bytes into human-readable size
func FormatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
