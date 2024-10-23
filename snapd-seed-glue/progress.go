// Copyright (C) 2024 Simon Quigley <tsimonq2@ubuntu.com>
//
// This program is free software; you can redistribute it and/or
// modify it under the terms of the GNU General Public License
// as published by the Free Software Foundation; either version 3
// of the License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.

package main

import (
    "fmt"
    "sync"
    "github.com/snapcore/snapd/progress"
)

var (
    progressReporter ProgressReporter
    progressTracker  *ProgressTracker
    globalDownloaded float64
    globalMu         sync.Mutex
    lastReported     int
)

type ProgressReporter interface {
    Report(percentage int, status string)
}

// VerboseProgressReporter formats progress updates
type VerboseProgressReporter struct{}

func (v *VerboseProgressReporter) Report(percentage int, status string) {
    fmt.Printf("%d\t%s\n", percentage, status)
}

// ProgressMeter tracks the download progress and implements the progress.Meter interface
type ProgressMeter struct {
    currentBytes float64
    isDelta      bool
    mu           sync.Mutex
    snapName     string
    snapVersion  string
    totalSize    float64
}

// Ensure ProgressMeter implements the progress.Meter interface
var _ progress.Meter = (*ProgressMeter)(nil)

// NewProgressMeter initializes a new ProgressMeter instance with the snap name
func NewProgressMeter(snapName string, snapVersion string, isDelta bool) *ProgressMeter {
    return &ProgressMeter{
        isDelta: isDelta,
        snapName: snapName,
        snapVersion: snapVersion,
        totalSize: snapSizeMap[snapName],
    }
}

// Start initializes the progress meter with the total size
func (pm *ProgressMeter) Start(label string, total float64) {
    pm.mu.Lock()
    defer pm.mu.Unlock()
    pm.totalSize = total

    // Update global total size
    globalMu.Lock()
    globalMu.Unlock()
}

// Set updates the progress based on the current value representing the number of bytes downloaded
func (pm *ProgressMeter) Set(value float64) {
    pm.mu.Lock()
    defer pm.mu.Unlock()

    delta := value - pm.currentBytes
    pm.currentBytes = value

    // Update global downloaded bytes
    globalMu.Lock()
    globalDownloaded += delta
    globalMu.Unlock()

    reportGlobalProgress(pm.snapName, pm.snapVersion, pm.isDelta)
}

// SetTotal sets the total size for the ProgressMeter
func (pm *ProgressMeter) SetTotal(total float64) {
    pm.mu.Lock()
    defer pm.mu.Unlock()
    pm.totalSize = total
}

// Finished marks the progress as complete
func (pm *ProgressMeter) Finished() {
    pm.mu.Lock()
    defer pm.mu.Unlock()

    delta := pm.totalSize - pm.currentBytes
    pm.currentBytes = pm.totalSize

    // Update global downloaded bytes
    globalMu.Lock()
    globalDownloaded += delta
    globalMu.Unlock()

    reportGlobalProgress(pm.snapName, pm.snapVersion, pm.isDelta)
}

// Write handles byte data to update progress based on the size of the data written
func (pm *ProgressMeter) Write(p []byte) (n int, err error) {
    pm.mu.Lock()
    defer pm.mu.Unlock()

    // Calculate the delta and update the current bytes
    delta := float64(len(p))
    pm.currentBytes += delta

    // Update global downloaded bytes
    globalMu.Lock()
    globalDownloaded += delta
    globalMu.Unlock()

    reportGlobalProgress(pm.snapName, pm.snapVersion, pm.isDelta)

    return len(p), nil
}

// reportGlobalProgress calculates and formats the overall progress percentage
func reportGlobalProgress(snapName string, snapVersion string, isDelta bool) {
    globalMu.Lock()
    defer globalMu.Unlock()

    if totalSnapSize == 0 {
        return
    }
    // Calculate the percentage within the range of 10 to 90
    percentage := int((globalDownloaded / totalSnapSize) * 80) + 10

    // Only print if there's a change in percentage to reduce output
    if percentage != lastReported {
        lastReported = percentage
        if isDelta {
            progressString := fmt.Sprintf("Downloading delta for snap %s %s", snapName, snapVersion)
            progressReporter.Report(percentage, progressString)
        } else {
            progressString := fmt.Sprintf("Downloading snap %s %s", snapName, snapVersion)
            progressReporter.Report(percentage, progressString)
        }
    }
}

// Spin shows indefinite activity; not used in this implementation
func (pm *ProgressMeter) Spin(msg string) {
    fmt.Printf("Spin: %s\n", msg)
}

// Notify formats notifications about the progress
func (pm *ProgressMeter) Notify(message string) {
    fmt.Printf("Notification: %s\n", message)
}

// ProgressTracker manages multiple steps of progress
type ProgressTracker struct {
    totalWeight     int
    completedWeight float64
    mu              sync.Mutex
    reporter        ProgressReporter
    steps           []*WeightedStep
    currentStep     int
}

// WeightedStep represents a step in a multi-step progress tracker
type WeightedStep struct {
    Weight   int
    Progress float64
    Status   string
}

// NewProgressTracker creates a new instance of ProgressTracker
func NewProgressTracker(reporter ProgressReporter) *ProgressTracker {
    return &ProgressTracker{
        reporter: reporter,
        steps:    []*WeightedStep{},
    }
}

// AddStep adds a new step to the progress tracker
func (pt *ProgressTracker) AddStep(weight int, status string) {
    pt.mu.Lock()
    defer pt.mu.Unlock()
    pt.steps = append(pt.steps, &WeightedStep{
        Weight: weight,
        Status: status,
    })
    pt.totalWeight += weight
}

// Start initializes the first step of the tracker
func (pt *ProgressTracker) Start() {
    pt.mu.Lock()
    defer pt.mu.Unlock()
    if len(pt.steps) > 0 {
        pt.currentStep = 0
        pt.steps[pt.currentStep].Progress = 0
        pt.reportProgress()
    }
}

// UpdateStepProgress updates the current step's progress
func (pt *ProgressTracker) UpdateStepProgress(progress float64) {
    pt.mu.Lock()
    defer pt.mu.Unlock()
    if pt.currentStep >= len(pt.steps) {
        return
    }
    step := pt.steps[pt.currentStep]
    if progress < 0 {
        progress = float64(int((globalDownloaded / totalSnapSize) * float64(step.Weight)))
    } else if progress < step.Progress {
        return
    }
    // Adjust delta calculation
    delta := (progress - step.Progress)
    pt.completedWeight += delta
    step.Progress = progress
    //pt.reportProgress()
}

func (pt *ProgressTracker) Finish(status string) {
    pt.mu.Lock()
    defer pt.mu.Unlock()
    if len(pt.steps) == 0 || pt.currentStep >= len(pt.steps) {
        return
    }
    step := pt.steps[pt.currentStep]
    if step.Progress < 100.0 {
        // Corrected calculation to prevent exceeding 100%
        remainingProgress := 100.0 - step.Progress
        delta := (remainingProgress * float64(step.Weight)) / 100.0
        pt.completedWeight += delta
        step.Progress = 100.0
    }
    percentage := pt.calculatePercentage()
    pt.reporter.Report(percentage, status)
    if pt.currentStep < len(pt.steps)-1 {
        pt.currentStep++
        pt.steps[pt.currentStep].Progress = 0
        pt.reportProgress()
    }
}

// NextStep moves to the next step in the progress tracker
func (pt *ProgressTracker) NextStep() {
    pt.mu.Lock()
    defer pt.mu.Unlock()
    if pt.currentStep < len(pt.steps)-1 {
        pt.currentStep++
        pt.steps[pt.currentStep].Progress = 0
        pt.reportProgress()
    }
}

// calculatePercentage calculates the overall progress as a percentage
func (pt *ProgressTracker) calculatePercentage() int {
    if pt.totalWeight == 0 {
        return 0
    }
    // Calculate percentage accurately and scale within 0-99 range
    percentage := int((pt.completedWeight / float64(pt.totalWeight)) * 100)
    if percentage > 99 {
        percentage = 99
    }
    return percentage
}

// reportProgress reports the current progress percentage to the reporter
func (pt *ProgressTracker) reportProgress() {
    percentage := pt.calculatePercentage()
    status := pt.steps[pt.currentStep].Status
    if percentage != lastReported {
        pt.reporter.Report(percentage, status)
        lastReported = percentage
    }
}

// InitProgress initializes the global progress tracker and sets up steps
func InitProgress() {
    progressReporter = &VerboseProgressReporter{}
    progressTracker = NewProgressTracker(progressReporter)
    progressTracker.AddStep(10, "Initialization")
    progressTracker.AddStep(80, "Downloading snaps")
    progressTracker.AddStep(10, "Verifying snaps")
    progressTracker.Start()
}
