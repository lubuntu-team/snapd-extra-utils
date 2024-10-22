package main

import (
    "encoding/base64"
    "encoding/hex"
    "fmt"
    "log"
    "os"
    "path/filepath"
    "strings"
    "time"

    "github.com/snapcore/snapd/asserts"
    "github.com/snapcore/snapd/snap"
    "github.com/snapcore/snapd/store"
    "gopkg.in/yaml.v3"
)

// downloadAssertions dynamically fetches the necessary assertions and saves them to a file.
// It ensures that the account-key assertion is written first in the .assert file.
func downloadAssertions(storeClient *store.Store, snapInfo *snap.Info, downloadDir string) error {
    // Define the path for the assertions file
    assertionsPath := filepath.Join(downloadDir, fmt.Sprintf("%s_%d.assert", snapInfo.SuggestedName, snapInfo.Revision.N))

    // Extract necessary fields from snapInfo
    snapSHA := snapInfo.Sha3_384
    snapID := snapInfo.SnapID
    publisherID := snapInfo.Publisher.ID
    series := "16" // Consider making this dynamic if possible

    // Define assertion types
    assertionTypes := map[string]*asserts.AssertionType{
        "snap-revision":    asserts.SnapRevisionType,
        "snap-declaration": asserts.SnapDeclarationType,
        "account-key":      asserts.AccountKeyType,
        "account":          asserts.AccountType,
    }

    // Open the assertions file for writing
    assertionsFile, err := os.Create(assertionsPath)
    if err != nil {
        return fmt.Errorf("failed to create assertions file: %w", err)
    }
    defer assertionsFile.Close()

    // Step 1: Fetch snap-declaration assertion
    snapDecl, err := storeClient.Assertion(assertionTypes["snap-declaration"], []string{series, snapID}, nil)
    if err != nil {
        return fmt.Errorf("failed to fetch snap-declaration assertion for snap %s: %w", snapInfo.SuggestedName, err)
    }

    // Step 2: Extract sign-key-sha3-384 from snap-declaration
    signKey, ok := snapDecl.Header("sign-key-sha3-384").(string)
    if !ok || signKey == "" {
        return fmt.Errorf("snap-declaration assertion missing 'sign-key-sha3-384' header for snap %s", snapInfo.SuggestedName)
    }

    // Step 3: Fetch account-key assertion using sign-key-sha3-384 (no decoding)
    accountKeyAssertion, err := storeClient.Assertion(assertionTypes["account-key"], []string{signKey}, nil)
    if err != nil {
        return fmt.Errorf("failed to fetch account-key assertion for snap %s: %w", snapInfo.SuggestedName, err)
    }

    // Step 4: Fetch account assertion using publisher-id
    accountAssertion, err := storeClient.Assertion(assertionTypes["account"], []string{publisherID}, nil)
    if err != nil {
        return fmt.Errorf("failed to fetch account assertion for snap %s: %w", snapInfo.SuggestedName, err)
    }

    // Step 5: Fetch snap-revision assertion
    snapSHA384Bytes, err := hex.DecodeString(snapSHA)
    if err != nil {
        return fmt.Errorf("error decoding SHA3-384 hex string for snap %s: %w", snapInfo.SuggestedName, err)
    }
    snapSHA384Base64 := base64.RawURLEncoding.EncodeToString(snapSHA384Bytes)
    //revisionKey := fmt.Sprintf("%s/global-upload", snapSHA384Base64)
    revisionKey := fmt.Sprintf("%s/", snapSHA384Base64)

    snapRevisionAssertion, err := storeClient.Assertion(assertionTypes["snap-revision"], []string{revisionKey}, nil)
    if err != nil {
        verboseLog("Failed to fetch snap-revision assertion for snap %s: %v", snapInfo.SuggestedName, err)
        // Proceeding without snap-revision might be acceptable based on your use-case
    }

    // Step 6: Write assertions in the desired order
    // 1. account-key
    writeAssertion("account-key", accountKeyAssertion, assertionsFile)

    // 2. account
    writeAssertion("account", accountAssertion, assertionsFile)

    // 3. snap-declaration
    writeAssertion("snap-declaration", snapDecl, assertionsFile)

    // 4. snap-revision (if fetched successfully)
    if snapRevisionAssertion != nil {
        writeAssertion("snap-revision", snapRevisionAssertion, assertionsFile)
    }

    verboseLog("Assertions downloaded and saved to: %s", assertionsPath)
    return nil
}

func writeAssertion(assertionType string, assertion asserts.Assertion, file *os.File) {
    fieldOrder := map[string][]string{
        "account-key": {
            "type", "authority-id", "revision", "public-key-sha3-384",
            "account-id", "name", "since", "body-length", "sign-key-sha3-384",
        },
        "account": {
            "type", "authority-id", "revision", "account-id", "display-name",
            "timestamp", "username", "validation", "sign-key-sha3-384",
        },
        "snap-declaration": {
            "type", "format", "authority-id", "revision", "series", "snap-id",
            "aliases", "auto-aliases", "plugs", "publisher-id", "slots",
            "snap-name", "timestamp", "sign-key-sha3-384",
        },
        "snap-revision": {
            "type", "authority-id", "snap-sha3-384", "developer-id",
            "provenance", "snap-id", "snap-revision", "snap-size",
            "timestamp", "sign-key-sha3-384",
        },
    }

    body := assertion.Body()
    bodyLength := len(body)
    headers := assertion.Headers()

    // Only write the account assertion if it is not Canonical
    if assertionType == "account" {
        value, exists := headers["username"]
        if exists && value == "canonical" {
            return
        }
    }

    // provenance seems to be a field only available in newer snap revisions
    // For snaps published in 2023 or earlier, do not include this field
    timestamp, exists := headers["timestamp"]
    if assertionType == "snap-revision" && exists {
        layout := time.RFC3339
        parsedTime, _ := time.Parse(layout, timestamp.(string))
        thresholdTime := time.Date(2023, time.December, 9, 0, 0, 0, 0, time.UTC)
        if parsedTime.Before(thresholdTime) || parsedTime.Equal(thresholdTime) {
            delete(headers, "provenance")
        }
    }

    // Write headers in the specified order
    for _, key := range fieldOrder[assertionType] {
        value, exists := headers[key]
        if !exists || value == "" {
            continue
        }
        if key == "type" {
            fmt.Fprintf(file, "%s: %s\n", key, assertionType)
        } else if key == "body-length" && bodyLength > 0 {
            file.WriteString(fmt.Sprintf("body-length: %d\n", bodyLength))
            continue
        } else if isComplexField(key) {
            fmt.Fprintf(file, "%s:\n", key)
            serializeComplexField(value, file)
        } else {
            fmt.Fprintf(file, "%s: %s\n", key, value)
        }
    }
    file.WriteString("\n")

    // Write the body if it exists
    if bodyLength > 0 {
        file.Write(body)
        file.WriteString("\n\n")
    }

    // Write the signature
    _, signature := assertion.Signature()
    file.Write(signature)
    if assertionType != "snap-revision" {
        file.WriteString("\n")
    }
}

func serializeComplexField(value interface{}, file *os.File) {
    var buf strings.Builder
    encoder := yaml.NewEncoder(&buf)
    encoder.SetIndent(2)
    defer encoder.Close()

    // Encode the value directly
    if err := encoder.Encode(value); err != nil {
        log.Fatalf("Error encoding YAML: %v", err)
    }

    // Write the serialized YAML to the file with proper indentation
    lines := strings.Split(buf.String(), "\n")
    for _, line := range lines {
        if line == "" {
            continue
        }
        line = strings.ReplaceAll(line, `"true"`, "true")
        line = strings.ReplaceAll(line, `"false"`, "false")
        line = strings.ReplaceAll(line, `'*'`, "*")
        // Check for dashes indicating list items
        if strings.HasPrefix(strings.TrimSpace(line), "-") && strings.Contains(line, ":") {
            before, after, found := strings.Cut(line, "- ")
            if found {
                file.WriteString(fmt.Sprintf("  %s-\n", before))
                if after != "" {
                    file.WriteString(fmt.Sprintf("    %s%s\n", before, after))
                }
            } else {
                file.WriteString(fmt.Sprintf("  %s-\n", line))
            }
        } else if strings.TrimSpace(line) != "" {
            // For any other non-empty lines, indent them correctly
            file.WriteString(fmt.Sprintf("  %s\n", line))
        }
    }
}

// isComplexField checks if a field is complex (nested) in YAML
func isComplexField(key string) bool {
    return key == "aliases" || key == "auto-aliases" || key == "plugs" || key == "slots" || key == "allow-installation" || key == "allow-connection"
}
