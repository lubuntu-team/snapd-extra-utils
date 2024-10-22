package main

import (
    "fmt"
    "os/exec"
    "path/filepath"
    "time"

    "github.com/snapcore/snapd/snap"
    "github.com/snapcore/snapd/store"
)

// downloadSnap downloads a snap file with retry logic
func downloadSnap(storeClient *store.Store, snapInfo *snap.Info, downloadPath string) error {
    downloadInfo := &snap.DownloadInfo{
        DownloadURL: snapInfo.DownloadURL,
    }

    pbar := NewProgressMeter(snapInfo.SuggestedName, snapInfo.Version, false)
    progressTracker.UpdateStepProgress(0)

    for attempts := 1; attempts <= 5; attempts++ {
        verboseLog("Attempt %d to download snap: %s", attempts, downloadPath)
        err := storeClient.Download(ctx, snapInfo.SnapID, downloadPath, downloadInfo, pbar, nil, nil)
        if err == nil {
            pbar.Finished()
            return nil // Successful download
        }
        if verbose {
            verboseLog("Attempt %d to download %s failed: %v", attempts, snapInfo.SuggestedName, err)
        } else if progressTracker != nil {
            progressTracker.UpdateStepProgress(0)
        }
        if attempts == 5 {
            return fmt.Errorf("snap download failed after 5 attempts: %v", err)
        }
    }
    return fmt.Errorf("snap download failed after 5 attempts")
}

// downloadSnapDeltaWithRetries downloads the delta file with retry logic and exponential backoff.
func downloadSnapDeltaWithRetries(storeClient *store.Store, delta *snap.DeltaInfo, result *store.SnapActionResult, deltaPath string, maxRetries int, snapName string) error {
    if !verbose {
        verboseLog("Downloading delta for %s", snapName)
    }
    var lastErr error
    backoff := 1 * time.Second

    for attempts := 1; attempts <= maxRetries; attempts++ {
        verboseLog("Attempt %d to download delta: %s", attempts, deltaPath)
        err := downloadSnapDelta(storeClient, delta, result, deltaPath)
        if err == nil {
            return nil
        }
        lastErr = err
        verboseLog("Attempt %d to download delta failed: %v", attempts, err)
        time.Sleep(backoff)
        backoff *= 2
    }
    return fmt.Errorf("delta download failed after %d attempts: %v", maxRetries, lastErr)
}

// downloadSnapDelta downloads the delta file.
func downloadSnapDelta(storeClient *store.Store, delta *snap.DeltaInfo, result *store.SnapActionResult, deltaPath string) error {
    verboseLog("Downloading delta from revision %d to %d from: %s", delta.FromRevision, delta.ToRevision, delta.DownloadURL)

    downloadInfo := &snap.DownloadInfo{
        DownloadURL: delta.DownloadURL,
        Size:        delta.Size,
        Sha3_384:    delta.Sha3_384,
    }

    // Use the SnapID from the associated SnapActionResult's Info
    snapID := result.Info.SnapID

    pbar := NewProgressMeter(result.Info.SuggestedName, result.Info.Version, true)
    progressTracker.UpdateStepProgress(0)

    // Download the delta file
    if err := storeClient.Download(ctx, snapID, deltaPath, downloadInfo, pbar, nil, nil); err != nil {
        progressTracker.UpdateStepProgress(0)
        return fmt.Errorf("delta download failed: %v", err)
    }
    verboseLog("Downloaded %s to %s", delta.DownloadURL, deltaPath)
    pbar.Finished()

    return nil
}

// downloadAndApplySnap handles the downloading and delta application process.
// It returns the snap information and an error if any.
func downloadAndApplySnap(storeClient *store.Store, result *store.SnapActionResult, snapsDir, assertionsDir string, currentSnap *store.CurrentSnap) (*snap.Info, error) {
    if result == nil || result.Info == nil {
        verboseLog("No updates available for snap. Skipping download and assertions.")
        return nil, nil
    }

    snapInfo := result.Info
    downloadPath := filepath.Join(snapsDir, fmt.Sprintf("%s_%d.snap", snapInfo.SuggestedName, snapInfo.Revision.N))

    // Check if a delta can be applied
    if currentSnap != nil && len(result.Deltas) > 0 {
        for _, delta := range result.Deltas {
            deltaPath := filepath.Join(snapsDir, fmt.Sprintf("%s_%d_to_%d.delta", snapInfo.SuggestedName, delta.FromRevision, delta.ToRevision))
            if err := downloadSnapDeltaWithRetries(storeClient, &delta, result, deltaPath, 5, snapInfo.SuggestedName); err == nil {
                oldSnapPath := filepath.Join(snapsDir, fmt.Sprintf("%s_%d.snap", snapInfo.SuggestedName, delta.FromRevision))
                if fileExists(oldSnapPath) {
                    if err := applyDelta(oldSnapPath, deltaPath, downloadPath); err == nil {
                        verboseLog("Delta applied successfully for snap %s", snapInfo.SuggestedName)
                        // Download assertions after successful snap download
                        if err := downloadAssertions(storeClient, snapInfo, assertionsDir); err != nil {
                            return nil, fmt.Errorf("failed to download assertions for snap %s: %w", snapInfo.SuggestedName, err)
                        }
                        return snapInfo, nil // Successful delta application
                    } else {
                        verboseLog("Failed to apply delta for snap %s: %v", snapInfo.SuggestedName, err)
                    }
                } else {
                    verboseLog("Old snap file %s does not exist. Cannot apply delta.", oldSnapPath)
                }
            } else {
                verboseLog("Attempt to download delta for snap %s failed: %v", snapInfo.SuggestedName, err)
            }
        }
    }

    // If no delta was applied or no deltas are available, fallback to downloading the full snap
    if err := downloadSnap(storeClient, snapInfo, downloadPath); err != nil {
        return nil, fmt.Errorf("failed to download snap %s: %w", snapInfo.SuggestedName, err)
    }

    // Download assertions after successful snap download
    if err := downloadAssertions(storeClient, snapInfo, assertionsDir); err != nil {
        return nil, fmt.Errorf("failed to download assertions for snap %s: %w", snapInfo.SuggestedName, err)
    }

    verboseLog("Downloaded and applied snap: %s, revision: %d", snapInfo.SuggestedName, snapInfo.Revision.N)
    return snapInfo, nil
}

// applyDelta applies the downloaded delta using xdelta3.
func applyDelta(oldSnapPath, deltaPath, newSnapPath string) error {
    verboseLog("Applying delta from %s to %s using %s", oldSnapPath, newSnapPath, deltaPath)

    cmd := exec.Command("xdelta3", "-d", "-s", oldSnapPath, deltaPath, newSnapPath)
    output, err := cmd.CombinedOutput()
    if err != nil {
        verboseLog("xdelta3 output: %s", string(output))
        return fmt.Errorf("failed to apply delta: %v - %s", err, string(output))
    }
    return nil
}
