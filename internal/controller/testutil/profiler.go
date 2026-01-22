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
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"
)

// TestProfiler collects CPU and memory profiles mapped to test cases
type TestProfiler struct {
	outputDir    string
	cpuFile      *os.File
	memSnapshots []MemorySnapshot
	testMarkers  []TestMarker
	mu           sync.Mutex
	startTime    time.Time
}

// MemorySnapshot captures memory state at a point in time
type MemorySnapshot struct {
	Timestamp   time.Time `json:"timestamp"`
	TestName    string    `json:"testName"`
	HeapAlloc   uint64    `json:"heapAlloc"`
	HeapSys     uint64    `json:"heapSys"`
	HeapObjects uint64    `json:"heapObjects"`
	GCPauses    uint64    `json:"gcPauses"`
}

// TestMarker marks the start and end of a test
type TestMarker struct {
	TestName  string        `json:"testName"`
	StartTime time.Time     `json:"startTime"`
	EndTime   time.Time     `json:"endTime"`
	Duration  time.Duration `json:"duration"`
	Passed    bool          `json:"passed"`
}

// NewTestProfiler creates a profiler that writes to the given directory
func NewTestProfiler(outputDir string) (*TestProfiler, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, err
	}

	return &TestProfiler{
		outputDir:    outputDir,
		memSnapshots: make([]MemorySnapshot, 0),
		testMarkers:  make([]TestMarker, 0),
		startTime:    time.Now(),
	}, nil
}

// StartCPUProfile begins CPU profiling
func (p *TestProfiler) StartCPUProfile() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	f, err := os.Create(filepath.Join(p.outputDir, "cpu.pprof"))
	if err != nil {
		return err
	}
	p.cpuFile = f
	return pprof.StartCPUProfile(f)
}

// StopCPUProfile stops CPU profiling
func (p *TestProfiler) StopCPUProfile() {
	p.mu.Lock()
	defer p.mu.Unlock()

	pprof.StopCPUProfile()
	if p.cpuFile != nil {
		p.cpuFile.Close()
	}
}

// SnapshotMemory captures current memory state
func (p *TestProfiler) SnapshotMemory(testName string) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	p.mu.Lock()
	defer p.mu.Unlock()

	p.memSnapshots = append(p.memSnapshots, MemorySnapshot{
		Timestamp:   time.Now(),
		TestName:    testName,
		HeapAlloc:   m.HeapAlloc,
		HeapSys:     m.HeapSys,
		HeapObjects: m.HeapObjects,
		GCPauses:    m.PauseTotalNs,
	})
}

// MarkTestStart marks the beginning of a test
func (p *TestProfiler) MarkTestStart(testName string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.testMarkers = append(p.testMarkers, TestMarker{
		TestName:  testName,
		StartTime: time.Now(),
	})

	// Also take a memory snapshot at test start
	p.snapshotMemoryUnlocked(testName + "_start")
}

// snapshotMemoryUnlocked is called when lock is already held
func (p *TestProfiler) snapshotMemoryUnlocked(testName string) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	p.memSnapshots = append(p.memSnapshots, MemorySnapshot{
		Timestamp:   time.Now(),
		TestName:    testName,
		HeapAlloc:   m.HeapAlloc,
		HeapSys:     m.HeapSys,
		HeapObjects: m.HeapObjects,
		GCPauses:    m.PauseTotalNs,
	})
}

// MarkTestEnd marks the end of a test
func (p *TestProfiler) MarkTestEnd(testName string, passed bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i := len(p.testMarkers) - 1; i >= 0; i-- {
		if p.testMarkers[i].TestName == testName {
			p.testMarkers[i].EndTime = time.Now()
			p.testMarkers[i].Duration = p.testMarkers[i].EndTime.Sub(p.testMarkers[i].StartTime)
			p.testMarkers[i].Passed = passed
			break
		}
	}

	// Also take a memory snapshot at test end
	p.snapshotMemoryUnlocked(testName + "_end")
}

// WriteMemoryProfile writes heap profile
func (p *TestProfiler) WriteMemoryProfile() error {
	f, err := os.Create(filepath.Join(p.outputDir, "heap.pprof"))
	if err != nil {
		return err
	}
	defer f.Close()

	runtime.GC() // Get accurate heap info
	return pprof.WriteHeapProfile(f)
}

// GetMemorySnapshots returns all memory snapshots
func (p *TestProfiler) GetMemorySnapshots() []MemorySnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()

	result := make([]MemorySnapshot, len(p.memSnapshots))
	copy(result, p.memSnapshots)
	return result
}

// GetTestMarkers returns all test markers
func (p *TestProfiler) GetTestMarkers() []TestMarker {
	p.mu.Lock()
	defer p.mu.Unlock()

	result := make([]TestMarker, len(p.testMarkers))
	copy(result, p.testMarkers)
	return result
}

// OutputDir returns the output directory path
func (p *TestProfiler) OutputDir() string {
	return p.outputDir
}

// GenerateReport creates an HTML report with graphs
func (p *TestProfiler) GenerateReport() error {
	return generateHTMLReport(p.outputDir, p.GetMemorySnapshots(), p.GetTestMarkers())
}
