// SPDX-License-Identifier: BSD-3-Clause
// SPDX-FileCopyrightText: 2025 Harald Sitter <sitter@kde.org>

package main

import (
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
