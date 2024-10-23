package main

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"

    "github.com/snapcore/snapd/store"
)

// cleanUpFiles removes partial, old, and orphaned snap and assertion files from the download and assertions directories.
func cleanUpFiles(snapsDir string, assertionsDir string) {
    verboseLog("Starting cleanup process...")

    // Load the seed.yaml data
    seedData := loadSeedData()

    // Create a map of valid snap and assertion files based on seed.yaml
    validSnaps := make(map[string]bool)
    validAssertions := make(map[string]bool)

    // Populate valid snaps and assertions from the seed.yaml data
    for _, snap := range seedData.Snaps {
        // Ensure correct extraction of revision
        revision := extractRevisionFromFile(snap.File)
        if revision == "" {
            verboseLog("Failed to extract revision from file name: %s", snap.File)
            continue
        }
        snapFileName := fmt.Sprintf("%s_%s.snap", snap.Name, revision)
        assertionFileName := fmt.Sprintf("%s_%s.assert", snap.Name, revision)

        validSnaps[snapFileName] = true
        validAssertions[assertionFileName] = true
    }

    // Log valid snaps and assertions
    verboseLog("Valid Snaps: %v", validSnaps)
    verboseLog("Valid Assertions: %v", validAssertions)

    // Remove outdated or partial snap files
    files, err := os.ReadDir(snapsDir)
    if err != nil {
        verboseLog("Error reading snaps directory for cleanup: %v", err)
    } else {
        for _, file := range files {
            filePath := filepath.Join(snapsDir, file.Name())
            if strings.HasSuffix(file.Name(), ".partial") || strings.HasSuffix(file.Name(), ".delta") {
                verboseLog("Removing partial/delta file: %s\n", filePath)
                if err := os.Remove(filePath); err != nil {
                    verboseLog("Failed to remove file %s: %v", filePath, err)
                } else if verbose {
                    verboseLog("Removed partial/delta file: %s", filePath)
                }
            } else if strings.HasSuffix(file.Name(), ".snap") {
                if !validSnaps[file.Name()] {
                    verboseLog("Removing outdated or orphaned snap file: %s\n", filePath)
                    if err := os.Remove(filePath); err != nil {
                        verboseLog("Failed to remove snap file %s: %v", filePath, err)
                    } else if verbose {
                        verboseLog("Removed snap file: %s", filePath)
                    }
                } else {
                    verboseLog("Snap file %s is valid and retained.\n", file.Name())
                }
            }
        }
    }

    // Remove orphaned assertion files
    files, err = os.ReadDir(assertionsDir)
    if err != nil {
        verboseLog("Error reading assertions directory for cleanup: %v", err)
    } else {
        for _, file := range files {
            filePath := filepath.Join(assertionsDir, file.Name())
            if strings.HasSuffix(file.Name(), ".assert") {
                if !validAssertions[file.Name()] {
                    verboseLog("Removing orphaned assertion file: %s\n", filePath)
                    if err := os.Remove(filePath); err != nil {
                        verboseLog("Failed to remove assertion file %s: %v", filePath, err)
                    } else if verbose {
                        verboseLog("Removed assertion file: %s", filePath)
                    }
                } else {
                    verboseLog("Assertion file %s is valid and retained.\n", file.Name())
                }
            }
        }
    }

    verboseLog("Cleanup process completed.")
}

// removeOrphanedFiles deletes the assertion and snap file corresponding to the removed snap.
func removeOrphanedFiles(snapName string, revision int, assertionsDir string, snapsDir string) {
    assertionFilePath := filepath.Join(assertionsDir, fmt.Sprintf("%s_%d.assert", snapName, revision))
    snapFilePath := filepath.Join(snapsDir, fmt.Sprintf("%s_%d.snap", snapName, revision))
    if fileExists(assertionFilePath) {
        err := os.Remove(assertionFilePath)
        if err != nil {
            verboseLog("Failed to remove assertion file %s: %v", assertionFilePath, err)
        } else {
            verboseLog("Removed assertion file: %s", assertionFilePath)
        }
    } else {
        verboseLog("Assertion file %s does not exist. No action taken.", assertionFilePath)
    }
    if fileExists(snapFilePath) {
        err := os.Remove(snapFilePath)
        if err != nil {
            verboseLog("Failed to remove snap file %s: %v", snapFilePath, err)
        } else {
            verboseLog("Removed snap file: %s", snapFilePath)
        }
    } else {
        verboseLog("Snap file %s does not exist. No action taken.", snapFilePath)
    }
}

// cleanUpCurrentSnaps removes snaps from currentSnaps that are not marked as required.
func cleanUpCurrentSnaps(assertionsDir string, snapsDir string) {
    var filteredSnaps []*store.CurrentSnap

    for _, snap := range currentSnaps {
        if requiredSnaps[snap.InstanceName] {
            filteredSnaps = append(filteredSnaps, snap)
        } else {
            verboseLog("Removing unnecessary snap: %s\n", snap.InstanceName)
            removeOrphanedFiles(snap.InstanceName, snap.Revision.N, assertionsDir, snapsDir)
        }
    }
    currentSnaps = filteredSnaps

    // Log the updated currentSnaps
    verboseLog("Filtered currentSnaps after cleanup:")
    for _, snap := range currentSnaps {
        verboseLog("- %s_%d.snap", snap.InstanceName, snap.Revision.N)
    }
}

// removeStateJson removes the state.json file if it exists
func removeStateJson(stateJsonPath string) {
    if _, err := os.Stat(stateJsonPath); err == nil {
        if err := os.Remove(stateJsonPath); err != nil {
            verboseLog("Failed to remove state.json: %v", err)
        } else if verbose {
            verboseLog("Removed state.json at %s", stateJsonPath)
        }
    }
}
