package main

import (
    "context"
    "flag"
    "log"
    "fmt"
    "path/filepath"
    "strings"

    "github.com/snapcore/snapd/snap"
    "github.com/snapcore/snapd/store"
)

// Seed structure for seed.yaml
type seed struct {
    Snaps []struct {
        Name    string `yaml:"name"`
        Channel string `yaml:"channel"`
        File    string `yaml:"file"`
    } `yaml:"snaps"`
}

var (
    ctx            = context.Background()
    storeClient    *store.Store
    verbose        bool
    currentSnaps   []*store.CurrentSnap
    requiredSnaps  map[string]bool
    processedSnaps = make(map[string]bool)
    snapSizeMap    = make(map[string]float64)
    totalSnapSize  float64
)

type SnapInfo struct {
    InstanceName string
    SnapID       string
    Revision     snap.Revision
}

func main() {
    // Override the default plug slot sanitizer
    snap.SanitizePlugsSlots = sanitizePlugsSlots

    // Initialize progress reporting
    InitProgress()
    totalSnapSize = 0

    // Initialize the store client
    storeClient = store.New(nil, nil)

    // Parse command-line flags
    var seedDirectory string
    flag.StringVar(&seedDirectory, "seed", "/var/lib/snapd/seed", "Specify the seed directory")
    flag.BoolVar(&verbose, "verbose", false, "Enable verbose output")
    flag.Parse()
    if !verbose {
        fmt.Printf("2\tLoading existing snaps...\n")
    }

    // Define directories based on the seed directory
    snapsDir := filepath.Join(seedDirectory, "snaps")
    assertionsDir := filepath.Join(seedDirectory, "assertions")
    seedYaml := filepath.Join(seedDirectory, "seed.yaml")

    // Setup directories and seed.yaml
    initializeDirectories(snapsDir, assertionsDir)
    initializeSeedYaml(seedYaml)

    // Load existing snaps from seed.yaml
    existingSnapsInYaml := loadExistingSnaps(seedYaml)

    // Populate currentSnaps based on existing snaps
    for snapName := range existingSnapsInYaml {
        snapInfo, err := getCurrentSnapInfo(assertionsDir, snapName)
        if err != nil {
            verboseLog("Failed to get info for existing snap %s: %v", snapName, err)
            continue
        }
        currentSnaps = append(currentSnaps, snapInfo)
    }

    // Process essential snaps
    requiredSnaps = map[string]bool{"snapd": true, "bare": true}
    for _, arg := range flag.Args() {
        requiredSnaps[arg] = true
    }
    if !verbose {
        fmt.Printf("4\tFetching information from the Snap Store...\n")
    }

    // Collect snaps to process
    snapsToProcess, err := collectSnapsToProcess(snapsDir, assertionsDir)
    if err != nil {
        log.Fatalf("Failed to collect snaps to process: %v", err)
    }

    progressTracker.Finish("Finished collecting snap info")

    // Calculate the number of snaps to download
    totalSnaps := len(snapsToProcess)
    if totalSnaps == 0 {
        verboseLog("No snaps to process.")
    } else {
        verboseLog("Total snaps to download: %d", totalSnaps)
    }

    // Initialize variables to track download progress
    completedSnaps := 0

    // Update "Downloading snaps" step to 0%
    progressTracker.UpdateStepProgress(0)

    // Process all the snaps that need updates
    for _, snapDetails := range snapsToProcess {
        if err := processSnap(snapDetails, snapsDir, assertionsDir); err != nil {
            log.Fatalf("Failed to process snap %s: %v", snapDetails.InstanceName, err)
        }
        completedSnaps++

        progressTracker.UpdateStepProgress(-1)
    }

    // Mark "Downloading snaps" as complete
    if totalSnaps > 0 {
        progressTracker.Finish("Downloading snaps completed")
    } else {
        // If no snaps to download, skip to finalizing
        progressTracker.NextStep()
    }

    // Remove unnecessary snaps after processing dependencies
    cleanUpCurrentSnaps(assertionsDir, snapsDir)

    // Update seed.yaml with the current required snaps
    if err := updateSeedYaml(snapsDir, seedYaml, currentSnaps); err != nil {
        log.Fatalf("Failed to update seed.yaml: %v", err)
    }

    // Perform cleanup and validation tasks
    removeStateJson(filepath.Join(seedDirectory, "..", "state.json"))
    ensureAssertions(assertionsDir)
    if err := validateSeed(seedYaml); err != nil {
        log.Fatalf("Seed validation failed: %v", err)
    }
    cleanUpFiles(snapsDir, assertionsDir, seedYaml)

    // Mark "Finalizing" as complete
    if progressTracker != nil {
        progressTracker.Finish("Cleanup and validation completed")
    }
}

// collectSnapsToProcess collects all snaps and their dependencies, returning only those that need updates
func collectSnapsToProcess(snapsDir, assertionsDir string) ([]SnapDetails, error) {
    var snapsToProcess []SnapDetails

    fallbackChannel := "latest/stable"
    for snapEntry := range requiredSnaps {
        // Extract channel if specified, default to "stable"
        parts := strings.SplitN(snapEntry, "=", 2)
        channel := "latest/stable/ubuntu-25.04"
        if len(parts) == 2 {
            channel = parts[1]
        }
        snapName := parts[0]

        // Collect snap dependencies and their statuses
        snapList, err := collectSnapDependencies(snapName, channel, fallbackChannel, snapsDir, assertionsDir)
        if err != nil {
            return nil, err
        }

        // Append only those snaps that need updates
        for _, snapDetails := range snapList {
            verboseLog("Processing snap: %s with result: %v", snapDetails.InstanceName, snapDetails.Result)
            if len(snapDetails.Result.Deltas) > 0 {
                for _, delta := range snapDetails.Result.Deltas {
                    snapSize := float64(delta.Size)
                    snapSizeMap[snapDetails.Result.Info.SuggestedName] = snapSize
                    totalSnapSize += snapSize
                }
            } else {
                snapSize := float64(snapDetails.Result.Info.Size)
                snapSizeMap[snapDetails.Result.Info.SuggestedName] = snapSize
                totalSnapSize += snapSize
            }
            snapsToProcess = append(snapsToProcess, snapDetails)
        }
    }

    return snapsToProcess, nil
}

// sanitizePlugsSlots is a placeholder function to sanitize plug slots in snap.Info
func sanitizePlugsSlots(info *snap.Info) {}

// verboseLog logs messages only when verbose mode is enabled
func verboseLog(format string, v ...interface{}) {
    if verbose {
        log.Printf(format, v...)
    }
}
