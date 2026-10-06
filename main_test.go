// SPDX-License-Identifier: BSD-3-Clause
// SPDX-FileCopyrightText: 2026 Harald Sitter <sitter@kde.org>

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetStorageUrl(t *testing.T) {
	url, err := getStorageURL("fixtures/os-release")
	assert.Nil(t, err, "getStorageUrl should not return an error")
	assert.NotNil(t, url, "URL should not be nil")
	assert.Equal(t, "https://storage.kde.org/kde-linux/testing-buildstream/sysupdate/v2", url.String(), "url does not match")

	url, err = getStorageURL("/dev/null")
	assert.NotNil(t, err, "getStorageUrl should return an error for invalid os-release path")
	assert.Nil(t, url, "URL should be nil for invalid os-release path")
}

func TestGetStoreUrl(t *testing.T) {
	url, err := getStoreURL("fixtures/os-release")
	assert.Nil(t, err, "getStoreUrl should not return an error")
	assert.Equal(t, "https://storage.kde.org/kde-linux/testing-buildstream/sysupdate/v2/store", url.String(), "url does not match")
}
