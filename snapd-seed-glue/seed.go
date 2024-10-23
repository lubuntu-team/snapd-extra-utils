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
    "io/ioutil"
    "log"
    "os"
    "strings"

    "gopkg.in/yaml.v3"
    "github.com/snapcore/snapd/store"
)

type seed struct {
    Snaps []struct {
        Name    string `yaml:"name"`
        Channel string `yaml:"channel"`
        File    string `yaml:"file"`
    } `yaml:"snaps"`
}

// getChannelName returns the channel name for a specific snap name
func getChannelName(snapName string) (string, error) {
    file, err := ioutil.ReadFile(seedYaml)
    if err != nil {
        return "", fmt.Errorf("failed to read seed.yaml: %w", err)
    }

    var seedData seed
    if err := yaml.Unmarshal(file, &seedData); err != nil {
        return "", fmt.Errorf("failed to parse seed.yaml: %w", err)
    }

    for _, snap := range seedData.Snaps {
        if snap.Name == snapName {
            return snap.Channel, nil
        }
    }
    return "", fmt.Errorf("snap %s not found in seed.yaml", snapName)
}

// initializeSeedYaml ensures that seed.yaml exists; if not, creates it.
func initializeSeedYaml() {
    if _, err := os.Stat(seedYaml); os.IsNotExist(err) {
        file, err := os.Create(seedYaml)
        if err != nil {
            log.Fatalf("Failed to create seed.yaml: %v", err)
        }
        defer file.Close()
        file.WriteString("snaps:\n")
    }
}

// loadSeedData loads seed data from seed.yaml
func loadSeedData() seed {
    file, err := ioutil.ReadFile(seedYaml)
    if err != nil {
        log.Fatalf("Failed to read seed.yaml: %v", err)
    }

    var seedData seed
    if err := yaml.Unmarshal(file, &seedData); err != nil {
        log.Fatalf("Failed to parse seed.yaml: %v", err)
    }

    return seedData
}

// loadExistingSnaps loads snaps from seed.yaml into a map
func loadExistingSnaps() map[string]bool {
    file, err := ioutil.ReadFile(seedYaml)
    if err != nil {
        log.Fatalf("Failed to read seed.yaml: %v", err)
    }

    var seedData seed
    if err := yaml.Unmarshal(file, &seedData); err != nil {
        log.Fatalf("Failed to parse seed.yaml: %v", err)
    }

    existing := make(map[string]bool)
    for _, snap := range seedData.Snaps {
        existing[snap.Name] = true
        verboseLog("Found %s in seed.yaml\n", snap.Name)
    }
    return existing
}

// updateSeedYaml updates the seed.yaml file with the current required snaps
func updateSeedYaml(snapsDir string, currentSnaps []*store.CurrentSnap) error {
    // Log the snaps to be written
    verboseLog("CurrentSnaps to be written to seed.yaml:")
    for _, snapInfo := range currentSnaps {
        verboseLog("- %s_%d.snap", snapInfo.InstanceName, snapInfo.Revision.N)
    }

    // Load existing seed data
    file, err := ioutil.ReadFile(seedYaml)
    if err != nil {
        return fmt.Errorf("failed to read seed.yaml: %w", err)
    }

    var seedData seed
    if err := yaml.Unmarshal(file, &seedData); err != nil {
        return fmt.Errorf("failed to parse seed.yaml: %w", err)
    }

    // Clear existing snaps
    seedData.Snaps = []struct {
        Name    string `yaml:"name"`
        Channel string `yaml:"channel"`
        File    string `yaml:"file"`
    }{}

    // Populate seedData with currentSnaps
    for _, snapInfo := range currentSnaps {
        snapFileName := fmt.Sprintf("%s_%d.snap", snapInfo.InstanceName, snapInfo.Revision.N)
        snapData := struct {
            Name    string `yaml:"name"`
            Channel string `yaml:"channel"`
            File    string `yaml:"file"`
        }{
            Name:    snapInfo.InstanceName,
            Channel: strings.Replace(snapInfo.TrackingChannel, "latest/", "", -1),
            File:    snapFileName,
        }
        seedData.Snaps = append(seedData.Snaps, snapData)
    }

    // Marshal the updated seedData back to YAML
    updatedYAML, err := yaml.Marshal(&seedData)
    if err != nil {
        return fmt.Errorf("failed to marshal updated seed data: %w", err)
    }

    // Write the updated YAML back to seed.yaml
    if err := ioutil.WriteFile(seedYaml, updatedYAML, 0644); err != nil {
        return fmt.Errorf("failed to write updated seed.yaml: %w", err)
    }

    verboseLog("Updated seed.yaml with current snaps.")
    return nil
}
