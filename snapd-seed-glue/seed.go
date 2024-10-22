package main

import (
    "fmt"
    "io/ioutil"
    "log"
    "os"

    "gopkg.in/yaml.v3"
    "github.com/snapcore/snapd/store"
)

// initializeSeedYaml ensures that seed.yaml exists; if not, creates it.
func initializeSeedYaml(seedYaml string) {
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
func loadSeedData(seedYaml string) seed {
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
func loadExistingSnaps(seedYaml string) map[string]bool {
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
func updateSeedYaml(snapsDir, seedYaml string, currentSnaps []*store.CurrentSnap) error {
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
            Channel: "stable", // Assuming 'stable' channel; modify as needed
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