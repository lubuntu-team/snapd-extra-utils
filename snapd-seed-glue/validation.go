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
    "encoding/base64"
    "fmt"
    "io/ioutil"
    "log"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
)

// validateSeed validates the seed using snap debug
func validateSeed(seedYaml string) error {
    cmd := exec.Command("snap", "debug", "validate-seed", seedYaml)
    output, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("validation failed with output: %s, error: %v", string(output), err)
    }
    verboseLog("Seed validation successful: %s", string(output))
    return nil
}

// ensureAssertions ensures that essential assertions are present
func ensureAssertions(assertionsDir string) {
    model := "generic-classic"
    brand := "generic"
    series := "16" // Hardcoded series as snap.Info does not have a Series field

    modelAssertionPath := filepath.Join(assertionsDir, "model")
    accountKeyAssertionPath := filepath.Join(assertionsDir, "account-key")
    accountAssertionPath := filepath.Join(assertionsDir, "account")

    // Check and generate model assertion
    if _, err := os.Stat(modelAssertionPath); os.IsNotExist(err) {
        output, err := exec.Command("snap", "known", "--remote", "model", "series="+series, "model="+model, "brand-id="+brand).CombinedOutput()
        if err != nil {
            log.Fatalf("Failed to fetch model assertion: %v, Output: %s", err, string(output))
        }
        if err := ioutil.WriteFile(modelAssertionPath, output, 0644); err != nil {
            log.Fatalf("Failed to write model assertion: %v", err)
        }
        verboseLog("Fetched and saved model assertion to %s", modelAssertionPath)
    }

    // Generate account-key assertion if not exists
    if _, err := os.Stat(accountKeyAssertionPath); os.IsNotExist(err) {
        signKeySha3 := grepPattern(modelAssertionPath, "sign-key-sha3-384: ")
        if signKeySha3 == "" {
            log.Fatalf("Failed to extract sign-key-sha3-384 from model assertion.")
        }
        output, err := exec.Command("snap", "known", "--remote", "account-key", "public-key-sha3-384="+signKeySha3).CombinedOutput()
        if err != nil {
            log.Fatalf("Failed to fetch account-key assertion: %v, Output: %s", err, string(output))
        }
        if err := ioutil.WriteFile(accountKeyAssertionPath, output, 0644); err != nil {
            log.Fatalf("Failed to write account-key assertion: %v", err)
        }
        verboseLog("Fetched and saved account-key assertion to %s", accountKeyAssertionPath)
    }

    // Generate account assertion if not exists
    if _, err := os.Stat(accountAssertionPath); os.IsNotExist(err) {
        accountId := grepPattern(accountKeyAssertionPath, "account-id: ")
        if accountId == "" {
            log.Fatalf("Failed to extract account-id from account-key assertion.")
        }
        output, err := exec.Command("snap", "known", "--remote", "account", "account-id="+accountId).CombinedOutput()
        if err != nil {
            log.Fatalf("Failed to fetch account assertion: %v, Output: %s", err, string(output))
        }
        if err := ioutil.WriteFile(accountAssertionPath, output, 0644); err != nil {
            log.Fatalf("Failed to write account assertion: %v", err)
        }
        verboseLog("Fetched and saved account assertion to %s", accountAssertionPath)
    }
}


// grepPattern extracts a specific pattern from a file
func grepPattern(filePath, pattern string) string {
    content, err := ioutil.ReadFile(filePath)
    if err != nil {
        log.Fatalf("Failed to read from file %s: %v", filePath, err)
    }
    lines := strings.Split(string(content), "\n")
    for _, line := range lines {
        if strings.Contains(line, pattern) {
            parts := strings.SplitN(line, ":", 2)
            if len(parts) == 2 {
                encodedValue := strings.TrimSpace(parts[1])
                // Check if the value is base64 encoded
                if decodedBytes, err := base64.StdEncoding.DecodeString(encodedValue); err == nil {
                    return string(decodedBytes)
                }
                return encodedValue
            }
        }
    }
    log.Fatalf("Pattern %s not found in file %s", pattern, filePath)
    return ""
}
