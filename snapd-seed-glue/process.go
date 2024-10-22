package main

import (
    "fmt"
    "os"
    "path/filepath"
    "strconv"
    "strings"

    "github.com/snapcore/snapd/snap"
    "github.com/snapcore/snapd/store"
)

// SnapDetails struct remains unchanged
type SnapDetails struct {
    InstanceName string
    Channel      string
    CurrentSnap  *store.CurrentSnap
    Result       *store.SnapActionResult
}

// collectSnapDependencies collects all dependencies for a given snap, marking them as requiredSnaps regardless of whether they need updates.
func collectSnapDependencies(snapName, channel, fallbackChannel, snapsDir, assertionsDir string) ([]SnapDetails, error) {
    var snapDetailsList []SnapDetails

    if processedSnaps[snapName] {
        verboseLog("Snap %s has already been processed. Skipping.", snapName)
        return snapDetailsList, nil // Added 0 for int64
    }

    oldSnapPath, oldSnap := findPreviousSnap(snapsDir, assertionsDir, snapName)

    var result *store.SnapActionResult
    var err error

    // Fetch or refresh snap information
    if oldSnap == nil || oldSnap.SnapID == "" || oldSnap.Revision.N == 0 {
        result, err = fetchOrRefreshSnapInfo(snapName, nil, channel)
        if err != nil {
            if strings.Contains(err.Error(), "no snap revision available as specified") {
                result, err = fetchOrRefreshSnapInfo(snapName, nil, fallbackChannel)
                if err != nil {
                    return nil, err
                }
            } else {
                return nil, err
            }
        }
    } else {
        result, err = fetchOrRefreshSnapInfo(snapName, oldSnap, channel)
        if err != nil {
            if strings.Contains(err.Error(), "snap has no updates available") {
                result, err = fetchOrRefreshSnapInfo(snapName, nil, channel)
                if err != nil {
                    return nil, err
                }
            } else if strings.Contains(err.Error(), "no snap revision available as specified") {
                result, err = fetchOrRefreshSnapInfo(snapName, oldSnap, fallbackChannel)
                if err != nil {
                    if strings.Contains(err.Error(), "snap has no updates available") {
                        result, err = fetchOrRefreshSnapInfo(snapName, nil, fallbackChannel)
                        if err != nil {
                            return nil, err
                        }
                    } else {
                        return nil, err
                    }
                }
            } else {
                return nil, err
            }
        }
    }

    if result == nil || result.Info == nil || result.Info.SnapID == "" || result.Info.Revision.N == 0 {
        return nil, fmt.Errorf("invalid snap information returned for %s: SnapID or Revision is missing", snapName) // Added 0 for int64
    }

    info := result.Info
    newSnap := &store.CurrentSnap{
        InstanceName: snapName,
        SnapID:       info.SnapID,
        Revision:     snap.Revision{N: info.Revision.N},
    }
    snapInCurrentSnaps, oldRevision := isSnapInCurrentSnaps(snapName)
    if snapInCurrentSnaps {
        removeSnapFromCurrentSnaps(snapName, oldRevision)
    }
    currentSnaps = append(currentSnaps, newSnap)
    processedSnaps[snapName] = true

    needsUpdate := (oldSnapPath == "" || oldSnap.Revision.N < info.Revision.N)

    if needsUpdate {
        snapDetailsList = append(snapDetailsList, SnapDetails{
            InstanceName: snapName,
            Channel:      channel,
            CurrentSnap:  newSnap,
            Result:       result,
        })
    } else {
        // Mark the snap as required even if no update is needed
        requiredSnaps[snapName] = true
    }

    // Safely handle dependencies
    tracker := snap.SimplePrereqTracker{}
    missingPrereqs := tracker.MissingProviderContentTags(info, nil)
    for prereq := range missingPrereqs {
        if !processedSnaps[prereq] {
            verboseLog("Collecting dependencies for prerequisite snap: %s for %s", prereq, snapName)
            prereqDetails, err := collectSnapDependencies(prereq, channel, fallbackChannel, snapsDir, assertionsDir)
            if err != nil {
                // Additional logging for dependency resolution issues
                verboseLog("Failed to collect dependencies for prerequisite %s for snap %s: %v", prereq, snapName, err)
                return nil, fmt.Errorf("failed to collect dependencies for prerequisite %s for snap %s: %v", prereq, snapName, err) // Added 0 for int64
            }
            snapDetailsList = append(snapDetailsList, prereqDetails...)
        }
    }

    // Also handle base snaps safely
    if info.Base != "" && !processedSnaps[info.Base] {
        verboseLog("Collecting dependencies for base snap: %s for %s", info.Base, snapName)
        baseDetails, err := collectSnapDependencies(info.Base, channel, fallbackChannel, snapsDir, assertionsDir)
        if err != nil {
            verboseLog("Failed to collect dependencies for base snap %s for snap %s: %v", info.Base, snapName, err)
            return nil, fmt.Errorf("failed to collect dependencies for base snap %s for snap %s: %v", info.Base, snapName, err) // Added 0 for int64
        }
        snapDetailsList = append(snapDetailsList, baseDetails...)
    }

    return snapDetailsList, nil
}

// processSnap handles the downloading and applying of a snap if updates are available.
// It gracefully handles the "no updates available" scenario and ensures dependencies are marked as required.
func processSnap(snapDetails SnapDetails, snapsDir, assertionsDir string) error {
    verboseLog("Processing snap: %s on channel: %s", snapDetails.InstanceName, snapDetails.Channel)

    // Proceed with downloading the snap (either full or delta) using downloadAndApplySnap
    snapInfo, err := downloadAndApplySnap(storeClient, snapDetails.Result, snapsDir, assertionsDir, snapDetails.CurrentSnap)
    if err != nil {
        return fmt.Errorf("failed to download snap %s: %w", snapDetails.InstanceName, err)
    }

    // Mark the snap as required after successful download and application
    requiredSnaps[snapDetails.InstanceName] = true
    verboseLog("Downloaded and applied snap: %s, revision: %d", snapInfo.SuggestedName, snapInfo.Revision.N)
    return nil
}

// fetchOrRefreshSnapInfo retrieves snap information and returns the SnapActionResult including deltas.
func fetchOrRefreshSnapInfo(snapName string, currentSnap *store.CurrentSnap, channel string) (*store.SnapActionResult, error) {
    var actions []*store.SnapAction
    var includeSnap []*store.CurrentSnap
    if currentSnap != nil {
        verboseLog("Crafting refresh SnapAction for %s", snapName)
        actions = append(actions, &store.SnapAction{
            Action:       "refresh",
            SnapID:       currentSnap.SnapID,
            InstanceName: snapName,
            Channel:      channel,
        })
        includeSnap = []*store.CurrentSnap{currentSnap}
    } else {
        verboseLog("Crafting install SnapAction for %s", snapName)
        actions = append(actions, &store.SnapAction{
            Action:       "install",
            InstanceName: snapName,
            Channel:      channel,
        })
        includeSnap = []*store.CurrentSnap{}
    }

    results, _, err := storeClient.SnapAction(ctx, includeSnap, actions, nil, nil, nil)
    if err != nil {
        verboseLog("SnapAction error for %s: %v", snapName, err)
        if strings.Contains(err.Error(), "snap has no updates available") && currentSnap != nil {
            return nil, err
        }
        return nil, fmt.Errorf("snap action failed for %s: %w", snapName, err)
    }

    if len(results) == 0 || results[0].Info == nil {
        return nil, fmt.Errorf("no snap info returned for snap %s", snapName)
    }

    result := &results[0]
    info := result.Info

    // Validate necessary fields in the snap information
    if info.SnapID == "" || info.Revision.N == 0 {
        return nil, fmt.Errorf("invalid snap information for %s: SnapID or Revision is missing", snapName)
    }

    verboseLog("Fetched latest snap info for %s: SnapID: %s, Revision: %d", snapName, info.SnapID, info.Revision.N)
    return result, nil
}

// findPreviousSnap locates the previous snap revision in the downloads directory.
func findPreviousSnap(downloadDir, assertionsDir, snapName string) (string, *store.CurrentSnap) {
    var currentSnap store.CurrentSnap
    files, err := os.ReadDir(downloadDir)
    if err != nil {
        verboseLog("Error reading directory: %v", err)
        return "", nil
    }

    var latestRevision int
    var latestSnapPath string

    for _, file := range files {
        if strings.HasPrefix(file.Name(), snapName+"_") && strings.HasSuffix(file.Name(), ".snap") {
            revisionStr := extractRevisionFromFile(file.Name())
            if revisionStr == "" {
                verboseLog("Failed to extract revision from file name: %s", file.Name())
                continue
            }
            revision, err := strconv.Atoi(revisionStr)
            if err != nil {
                verboseLog("Failed to parse revision number for file %s: %v", file.Name(), err)
                continue
            }
            verboseLog("Found %s with revision %d", file.Name(), revision)

            if revision > latestRevision {
                latestRevision = revision
                latestSnapPath = filepath.Join(downloadDir, file.Name())

                // Parse the corresponding assertion file
                assertFilePath := filepath.Join(assertionsDir, strings.Replace(file.Name(), ".snap", ".assert", 1))
                currentSnap = parseSnapInfo(assertFilePath, snapName)
                currentSnap.Revision.N = revision
            }
        }
    }

    if latestSnapPath != "" {
        return latestSnapPath, &currentSnap
    }
    return "", nil
}
