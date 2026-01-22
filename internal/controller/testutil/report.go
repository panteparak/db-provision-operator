//go:build integration

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

package testutil

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"time"
)

// ReportData holds all data for the HTML report
type ReportData struct {
	TotalTests     int
	PassedTests    int
	FailedTests    int
	TotalDuration  string
	PeakMemoryMB   float64
	Tests          []TestReportEntry
	MemoryDataJSON template.JS
	TestDataJSON   template.JS
}

// TestReportEntry represents a single test in the report
type TestReportEntry struct {
	TestName    string
	Duration    string
	Passed      bool
	MemoryDelta string
}

// MemoryDataPoint represents a point in the memory chart
type MemoryDataPoint struct {
	Timestamp string  `json:"timestamp"`
	HeapMB    float64 `json:"heapMB"`
	TestName  string  `json:"testName"`
}

// TestDataPoint represents a test in the duration chart
type TestDataPoint struct {
	Name        string  `json:"name"`
	DurationMs  int64   `json:"durationMs"`
	Passed      bool    `json:"passed"`
	MemoryDelta float64 `json:"memoryDelta"`
	StartIndex  int     `json:"startIndex"`
}

const reportTemplate = `<!DOCTYPE html>
<html>
<head>
    <title>Integration Test Performance Report</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <style>
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; margin: 0; padding: 20px; background: #f5f5f5; }
        .container { max-width: 1400px; margin: 0 auto; }
        h1 { color: #333; border-bottom: 2px solid #4CAF50; padding-bottom: 10px; }
        h2 { color: #555; margin-top: 30px; }
        .chart-container { width: 100%; background: white; border-radius: 8px; padding: 20px; margin: 20px 0; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        .summary { background: white; padding: 20px; border-radius: 8px; margin-bottom: 20px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 20px; }
        .summary-card { text-align: center; padding: 15px; border-radius: 8px; }
        .summary-card.total { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; }
        .summary-card.passed { background: linear-gradient(135deg, #11998e 0%, #38ef7d 100%); color: white; }
        .summary-card.failed { background: linear-gradient(135deg, #eb3349 0%, #f45c43 100%); color: white; }
        .summary-card.duration { background: linear-gradient(135deg, #2193b0 0%, #6dd5ed 100%); color: white; }
        .summary-card.memory { background: linear-gradient(135deg, #f093fb 0%, #f5576c 100%); color: white; }
        .summary-card h3 { margin: 0 0 10px 0; font-size: 14px; opacity: 0.9; }
        .summary-card .value { font-size: 28px; font-weight: bold; }
        .test-table { width: 100%; border-collapse: collapse; background: white; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        .test-table th { background: #667eea; color: white; padding: 12px 15px; text-align: left; font-weight: 600; }
        .test-table td { padding: 12px 15px; border-bottom: 1px solid #eee; }
        .test-table tr:hover { background: #f8f9fa; }
        .status-passed { color: #11998e; font-weight: bold; }
        .status-failed { color: #eb3349; font-weight: bold; }
        .download-section { margin: 20px 0; display: flex; gap: 10px; flex-wrap: wrap; }
        .download-btn { background: #667eea; color: white; padding: 12px 24px; border: none; border-radius: 6px; cursor: pointer; font-size: 14px; transition: all 0.3s ease; text-decoration: none; display: inline-block; }
        .download-btn:hover { background: #5a6fd6; transform: translateY(-2px); box-shadow: 0 4px 12px rgba(102, 126, 234, 0.4); }
        .download-btn.secondary { background: #6c757d; }
        .download-btn.secondary:hover { background: #5a6268; }
        canvas { max-height: 400px; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Integration Test Performance Report</h1>

        <div class="summary">
            <div class="summary-card total">
                <h3>Total Tests</h3>
                <div class="value">{{.TotalTests}}</div>
            </div>
            <div class="summary-card passed">
                <h3>Passed</h3>
                <div class="value">{{.PassedTests}}</div>
            </div>
            <div class="summary-card failed">
                <h3>Failed</h3>
                <div class="value">{{.FailedTests}}</div>
            </div>
            <div class="summary-card duration">
                <h3>Total Duration</h3>
                <div class="value">{{.TotalDuration}}</div>
            </div>
            <div class="summary-card memory">
                <h3>Peak Memory</h3>
                <div class="value">{{printf "%.1f" .PeakMemoryMB}} MB</div>
            </div>
        </div>

        <div class="download-section">
            <button class="download-btn" onclick="downloadCSV()">Download CSV</button>
            <button class="download-btn secondary" onclick="downloadJSON()">Download JSON</button>
            <a class="download-btn secondary" href="cpu.pprof" download>CPU Profile (.pprof)</a>
            <a class="download-btn secondary" href="heap.pprof" download>Heap Profile (.pprof)</a>
        </div>

        <div class="chart-container">
            <h2>Memory Usage Over Time</h2>
            <canvas id="memoryChart"></canvas>
        </div>

        <div class="chart-container">
            <h2>Test Duration</h2>
            <canvas id="durationChart"></canvas>
        </div>

        <h2>Test Results</h2>
        <table class="test-table">
            <thead>
                <tr>
                    <th>Test Name</th>
                    <th>Duration</th>
                    <th>Status</th>
                    <th>Memory Delta</th>
                </tr>
            </thead>
            <tbody>
                {{range .Tests}}
                <tr>
                    <td>{{.TestName}}</td>
                    <td>{{.Duration}}</td>
                    <td class="{{if .Passed}}status-passed{{else}}status-failed{{end}}">{{if .Passed}}PASSED{{else}}FAILED{{end}}</td>
                    <td>{{.MemoryDelta}}</td>
                </tr>
                {{end}}
            </tbody>
        </table>
    </div>

    <script>
        const memoryData = {{.MemoryDataJSON}};
        const testData = {{.TestDataJSON}};

        // Memory chart
        new Chart(document.getElementById('memoryChart'), {
            type: 'line',
            data: {
                labels: memoryData.map((d, i) => i),
                datasets: [{
                    label: 'Heap Allocated (MB)',
                    data: memoryData.map(d => d.heapMB),
                    borderColor: 'rgb(102, 126, 234)',
                    backgroundColor: 'rgba(102, 126, 234, 0.1)',
                    fill: true,
                    tension: 0.3
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    tooltip: {
                        callbacks: {
                            title: (items) => memoryData[items[0].dataIndex]?.testName || '',
                            label: (item) => 'Heap: ' + item.raw.toFixed(2) + ' MB'
                        }
                    }
                },
                scales: {
                    y: {
                        beginAtZero: true,
                        title: { display: true, text: 'Memory (MB)' }
                    },
                    x: {
                        title: { display: true, text: 'Sample Index' }
                    }
                }
            }
        });

        // Duration chart
        new Chart(document.getElementById('durationChart'), {
            type: 'bar',
            data: {
                labels: testData.map(d => d.name.length > 30 ? d.name.substring(0, 30) + '...' : d.name),
                datasets: [{
                    label: 'Duration (ms)',
                    data: testData.map(d => d.durationMs),
                    backgroundColor: testData.map(d => d.passed ? 'rgba(17, 153, 142, 0.7)' : 'rgba(235, 51, 73, 0.7)'),
                    borderColor: testData.map(d => d.passed ? 'rgb(17, 153, 142)' : 'rgb(235, 51, 73)'),
                    borderWidth: 1
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    tooltip: {
                        callbacks: {
                            title: (items) => testData[items[0].dataIndex]?.name || '',
                            label: (item) => 'Duration: ' + item.raw + ' ms'
                        }
                    }
                },
                scales: {
                    y: {
                        beginAtZero: true,
                        title: { display: true, text: 'Duration (ms)' }
                    },
                    x: {
                        ticks: { maxRotation: 45, minRotation: 45 }
                    }
                }
            }
        });

        function downloadCSV() {
            let csv = 'Test Name,Duration (ms),Status,Memory Delta (MB)\n';
            testData.forEach(t => {
                csv += '"' + t.name.replace(/"/g, '""') + '",' + t.durationMs + ',' + (t.passed ? 'PASSED' : 'FAILED') + ',' + t.memoryDelta.toFixed(2) + '\n';
            });
            download('test-results.csv', csv, 'text/csv');
        }

        function downloadJSON() {
            const data = {
                summary: {
                    totalTests: {{.TotalTests}},
                    passedTests: {{.PassedTests}},
                    failedTests: {{.FailedTests}},
                    totalDuration: '{{.TotalDuration}}',
                    peakMemoryMB: {{.PeakMemoryMB}}
                },
                tests: testData,
                memorySnapshots: memoryData
            };
            download('test-results.json', JSON.stringify(data, null, 2), 'application/json');
        }

        function download(filename, content, mimeType) {
            const blob = new Blob([content], {type: mimeType});
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = filename;
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            URL.revokeObjectURL(url);
        }
    </script>
</body>
</html>`

func generateHTMLReport(outputDir string, snapshots []MemorySnapshot, markers []TestMarker) error {
	// Prepare memory data for chart
	memoryData := make([]MemoryDataPoint, len(snapshots))
	var peakMemory uint64
	for i, s := range snapshots {
		memoryData[i] = MemoryDataPoint{
			Timestamp: s.Timestamp.Format(time.RFC3339),
			HeapMB:    float64(s.HeapAlloc) / 1024 / 1024,
			TestName:  s.TestName,
		}
		if s.HeapAlloc > peakMemory {
			peakMemory = s.HeapAlloc
		}
	}

	// Prepare test data for chart
	testData := make([]TestDataPoint, 0, len(markers))
	var totalDuration time.Duration
	passedCount := 0
	failedCount := 0

	// Create a map of start/end memory for calculating deltas
	memoryMap := make(map[string]uint64)
	for _, s := range snapshots {
		memoryMap[s.TestName] = s.HeapAlloc
	}

	for i, m := range markers {
		if m.EndTime.IsZero() {
			continue // Skip incomplete tests
		}

		totalDuration += m.Duration
		if m.Passed {
			passedCount++
		} else {
			failedCount++
		}

		// Calculate memory delta
		startMem := memoryMap[m.TestName+"_start"]
		endMem := memoryMap[m.TestName+"_end"]
		memDeltaMB := float64(int64(endMem)-int64(startMem)) / 1024 / 1024

		testData = append(testData, TestDataPoint{
			Name:        m.TestName,
			DurationMs:  m.Duration.Milliseconds(),
			Passed:      m.Passed,
			MemoryDelta: memDeltaMB,
			StartIndex:  i,
		})
	}

	// Prepare test entries for table
	tests := make([]TestReportEntry, len(testData))
	for i, td := range testData {
		sign := ""
		if td.MemoryDelta > 0 {
			sign = "+"
		}
		tests[i] = TestReportEntry{
			TestName:    td.Name,
			Duration:    time.Duration(td.DurationMs * int64(time.Millisecond)).String(),
			Passed:      td.Passed,
			MemoryDelta: sign + formatFloat(td.MemoryDelta) + " MB",
		}
	}

	// Marshal data to JSON for JavaScript
	memoryDataJSON, _ := json.Marshal(memoryData)
	testDataJSON, _ := json.Marshal(testData)

	data := ReportData{
		TotalTests:     passedCount + failedCount,
		PassedTests:    passedCount,
		FailedTests:    failedCount,
		TotalDuration:  totalDuration.Round(time.Millisecond).String(),
		PeakMemoryMB:   float64(peakMemory) / 1024 / 1024,
		Tests:          tests,
		MemoryDataJSON: template.JS(memoryDataJSON),
		TestDataJSON:   template.JS(testDataJSON),
	}

	tmpl, err := template.New("report").Parse(reportTemplate)
	if err != nil {
		return err
	}

	f, err := os.Create(filepath.Join(outputDir, "report.html"))
	if err != nil {
		return err
	}
	defer f.Close()

	return tmpl.Execute(f, data)
}

func formatFloat(f float64) string {
	return fmt.Sprintf("%.2f", f)
}
