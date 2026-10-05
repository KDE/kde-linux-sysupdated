// SPDX-License-Identifier: BSD-3-Clause
// SPDX-FileCopyrightText: 2026 Harald Sitter <sitter@kde.org>

package main

import (
	"os"
	"testing"
)

func TestPathFromUrl(t *testing.T) {
	path, err := pathFromUrl("https://storage.kde.org/kde-linux/%W/sysupdate/v2/")
	if err != nil {
		t.Fatalf("Failed to get path from URL: %s", err)
	}
	if path != "/kde-linux/%W/sysupdate/v2/" {
		t.Fatalf("Unexpected path: %s", path)
	}

	_, err = pathFromUrl("https://storage.kde.org")
	if err == nil {
		t.Fatalf("Expected error for invalid URL format, got nil")
	}

	_, err = pathFromUrl("storage.kde.org/kde-linux/%W/sysupdate/v2/")
	if err == nil {
		t.Fatalf("Expected error for invalid URL format, got nil")
	}
}

func TestPathFromTransferFile(t *testing.T) {
	// Create a temporary transfer file for testing
	tempFile, err := os.CreateTemp("", "test.transfer")
	if err != nil {
		t.Fatalf("Failed to create temporary file: %s", err)
	}
	defer os.Remove(tempFile.Name())

	// Write a sample unit file content to the temporary file
	unitContent := `[Source]
Path=https://storage.kde.org/kde-linux/%W/sysupdate/v2/
`
	if _, err := tempFile.WriteString(unitContent); err != nil {
		t.Fatalf("Failed to write to temporary file: %s", err)
	}
	tempFile.Close()

	path, err := pathFromTransferFile(tempFile.Name())
	if err != nil {
		t.Fatalf("Failed to get path from transfer file: %s", err)
	}
	if path != "/kde-linux/%W/sysupdate/v2/" {
		t.Fatalf("Unexpected path: %s", path)
	}
}
