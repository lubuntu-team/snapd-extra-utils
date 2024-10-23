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
    "bufio"
    "encoding/hex"
    "fmt"
    "io"
    "log"
    "os"
    "path/filepath"
    "strconv"
    "strings"

    "github.com/snapcore/snapd/snap"
    "github.com/snapcore/snapd/store"
    "golang.org/x/crypto/sha3"
)

// verifyChecksum calculates the SHA3-384 checksum of a file and compares it with the expected checksum.
func verifyChecksum(filePath, expectedChecksum string) (bool, error) {
    file, err := os.Open(filePath)
    if err != nil {
        return false, fmt.Errorf("failed to open file for checksum verification: %w", err)
    }
    defer file.Close()

    hash := sha3.New384()
    if _, err := io.Copy(hash, file); err != nil {
        return false, fmt.Errorf("failed to calculate checksum: %w", err)
    }

    calculatedChecksum := hex.EncodeToString(hash.Sum(nil))
    return strings.EqualFold(calculatedChecksum, expectedChecksum), nil
}

// extractRevisionFromFile extracts the revision number from a file name by splitting at the last underscore.
func extractRevisionFromFile(fileName string) string {
    lastUnderscore := strings.LastIndex(fileName, "_")
    if lastUnderscore == -1 {
        return ""
    }
    revisionWithSuffix := fileName[lastUnderscore+1:]
    // Handle both .snap and .assert suffixes
    if strings.HasSuffix(revisionWithSuffix, ".snap") {
        return strings.TrimSuffix(revisionWithSuffix, ".snap")
    } else if strings.HasSuffix(revisionWithSuffix, ".assert") {
        return strings.TrimSuffix(revisionWithSuffix, ".assert")
    }
    return revisionWithSuffix
}

// fileExists checks if a file exists and is not a directory before we use it
func fileExists(filename string) bool {
    info, err := os.Stat(filename)
    if os.IsNotExist(err) {
        return false
    }
    return !info.IsDir()
}

// parseSnapInfo extracts snap details from the assertion file and returns the snap's current revision.
func parseSnapInfo(assertFilePath, snapName string) store.CurrentSnap {
    currentSnap := store.CurrentSnap{InstanceName: snapName}
    file, err := os.Open(assertFilePath)
    if err != nil {
        verboseLog("Failed to read assertion file: %v\n", err)
        return currentSnap
    }
    defer file.Close()

    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        line := strings.TrimSpace(scanner.Text())
        if strings.HasPrefix(line, "snap-id:") {
            currentSnap.SnapID = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
        }
        if strings.HasPrefix(line, "snap-revision:") {
            revisionStr := strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
            revision, err := strconv.Atoi(revisionStr)
            if err != nil {
                verboseLog("Failed to parse snap-revision for snap %s: %v", snapName, err)
                continue
            }
            currentSnap.Revision.N = revision
        }
    }

    // Validate that required fields are present
    if currentSnap.SnapID == "" || currentSnap.Revision.N == 0 {
        verboseLog("Incomplete snap info in assertion file: %s", assertFilePath)
    }

    return currentSnap
}

// isSnapInCurrentSnaps checks if a snap is already in currentSnaps
func isSnapInCurrentSnaps(snapName string) (bool, snap.Revision) {
    for _, snap := range currentSnaps {
        if snap.InstanceName == snapName {
            return true, snap.Revision
        }
    }
    return false, snap.Revision{}
}

func removeSnapFromCurrentSnaps(snapName string, revision snap.Revision) {
    for i := 0; i < len(currentSnaps); i++ {
        if currentSnaps[i].Revision == revision {
            // Remove the element at index i
            currentSnaps = append(currentSnaps[:i], currentSnaps[i+1:]...)
            return
        }
    }
}

// initializeDirectories ensures that the snaps and assertions directories exist
func initializeDirectories(snapsDir, assertionsDir string) {
    if err := os.MkdirAll(snapsDir, 0755); err != nil {
        log.Fatalf("Failed to create snaps directory: %v", err)
    }
    if err := os.MkdirAll(assertionsDir, 0755); err != nil {
        log.Fatalf("Failed to create assertions directory: %v", err)
    }
}

// getCurrentSnapInfo retrieves current snap information from assertions
func getCurrentSnapInfo(assertionsDir, snapName string) (*store.CurrentSnap, error) {
    assertionFiles, err := filepath.Glob(filepath.Join(assertionsDir, fmt.Sprintf("%s_*.assert", snapName)))
    if err != nil || len(assertionFiles) == 0 {
        return nil, fmt.Errorf("no assertion file found for snap: %s", snapName)
    }

    assertionFile := assertionFiles[0]
    currentSnap := parseSnapInfo(assertionFile, snapName)
    if currentSnap.SnapID == "" || currentSnap.Revision.N == 0 {
        return nil, fmt.Errorf("incomplete snap info in assertion file for snap: %s", snapName)
    }
    verboseLog("Found snap info for %s: SnapID: %s, Revision: %d", snapName, currentSnap.SnapID, currentSnap.Revision.N)
    return &currentSnap, nil
}

// verifySnapIntegrity verifies the integrity of a snap file
func verifySnapIntegrity(filePath, expectedChecksum string) bool {
    checksumMatches, err := verifyChecksum(filePath, expectedChecksum)
    if err != nil {
        verboseLog("Checksum verification failed for %s: %v", filePath, err)
        return false
    }
    return checksumMatches
}

// Get the raw VERSION_ID from /etc/os-release to use for branch detection
func getVersionID() (string, error) {
    file, err := os.Open("/etc/os-release")
    if err != nil {
        return "", fmt.Errorf("failed to open /etc/os-release: %w", err)
    }
    defer file.Close()

    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        line := scanner.Text()
        if strings.HasPrefix(line, "VERSION_ID=") {
            // Remove the prefix and any surrounding quotes
            versionID := strings.Trim(strings.SplitN(line, "=", 2)[1], `"`)
            return versionID, nil
        }
    }

    if err := scanner.Err(); err != nil {
        return "", fmt.Errorf("error reading /etc/os-release: %w", err)
    }

    return "", fmt.Errorf("VERSION_ID not found in /etc/os-release")
}
