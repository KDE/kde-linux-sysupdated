// SPDX-License-Identifier: BSD-3-Clause
// SPDX-FileCopyrightText: 2026 Harald Sitter <sitter@kde.org>

package main

import (
	"net"
	"net/http"
	"net/url"
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

type HTTPContextHandler struct {
}

func (h *HTTPContextHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/fail":
		w.WriteHeader(http.StatusInternalServerError)
		return
	case "/cdn-hit":
		w.Header().Set("X-77-Cache", "HIT")
		w.WriteHeader(http.StatusOK)
		return
	case "/cdn-miss":
		w.Header().Set("X-77-Cache", "MISS")
		w.WriteHeader(http.StatusOK)
		return
	case "/mirror":
		w.WriteHeader(http.StatusOK)
		return
	case "/nodelta":
		w.Header().Set("X-KDE-Delta", "false")
		w.WriteHeader(http.StatusOK)
		return
	case "/nostore":
		w.Header().Set("X-KDE-Store", "false")
		w.WriteHeader(http.StatusOK)
		return
	}
	panic("unexpected request path" + r.URL.Path)
}

func TestNewHTTPContext(t *testing.T) {
	server := http.Server{
		Addr:    "127.0.0.1:0",
		Handler: &HTTPContextHandler{},
	}
	listener, err := net.Listen("tcp", server.Addr)
	assert.NoError(t, err, "Failed to start listener")
	go server.Serve(listener)
	defer server.Close()

	{
		parsedURL, err := url.Parse("http://" + listener.Addr().String() + "/fail")
		assert.NoError(t, err, "URL parsing should not return an error")
		assert.NotNil(t, parsedURL, "Parsed URL should not be nil")

		context, err := newHTTPContext(parsedURL)
		assert.NotNil(t, err, "newHTTPContext should return an error")
		assert.NotNil(t, context, "Context should not be nil")

		assert.Equal(t, context, HTTPContext{})
	}

	{
		parsedURL, err := url.Parse("http://" + listener.Addr().String() + "/cdn-hit")
		assert.NoError(t, err, "URL parsing should not return an error")
		assert.NotNil(t, parsedURL, "Parsed URL should not be nil")

		context, err := newHTTPContext(parsedURL)
		assert.NoError(t, err, "newHTTPContext should not return an error")
		assert.NotNil(t, context, "Context should not be nil")

		assert.Equal(t, context, HTTPContext{URLType: CDNURLType, Cached: true, AllowDelta: true, AllowStore: true})

	}

	{
		parsedURL, err := url.Parse("http://" + listener.Addr().String() + "/cdn-miss")
		assert.NoError(t, err, "URL parsing should not return an error")
		assert.NotNil(t, parsedURL, "Parsed URL should not be nil")

		context, err := newHTTPContext(parsedURL)
		assert.NoError(t, err, "newHTTPContext should not return an error")
		assert.NotNil(t, context, "Context should not be nil")

		assert.Equal(t, context, HTTPContext{URLType: CDNURLType, Cached: false, AllowDelta: true, AllowStore: true})
	}

	{
		parsedURL, err := url.Parse("http://" + listener.Addr().String() + "/mirror")
		assert.NoError(t, err, "URL parsing should not return an error")
		assert.NotNil(t, parsedURL, "Parsed URL should not be nil")

		context, err := newHTTPContext(parsedURL)
		assert.NoError(t, err, "newHTTPContext should not return an error")
		assert.NotNil(t, context, "Context should not be nil")

		assert.Equal(t, context, HTTPContext{URLType: MirrorURLType, Cached: true, AllowDelta: true, AllowStore: true})
	}

	{
		parsedURL, err := url.Parse("http://" + listener.Addr().String() + "/nodelta")
		assert.NoError(t, err, "URL parsing should not return an error")
		assert.NotNil(t, parsedURL, "Parsed URL should not be nil")

		context, err := newHTTPContext(parsedURL)
		assert.NoError(t, err, "newHTTPContext should not return an error")
		assert.NotNil(t, context, "Context should not be nil")

		assert.Equal(t, context, HTTPContext{URLType: MirrorURLType, Cached: true, AllowDelta: false, AllowStore: false})
	}

	{
		parsedURL, err := url.Parse("http://" + listener.Addr().String() + "/nostore")
		assert.NoError(t, err, "URL parsing should not return an error")
		assert.NotNil(t, parsedURL, "Parsed URL should not be nil")

		context, err := newHTTPContext(parsedURL)
		assert.NoError(t, err, "newHTTPContext should not return an error")
		assert.NotNil(t, context, "Context should not be nil")

		assert.Equal(t, context, HTTPContext{URLType: MirrorURLType, Cached: true, AllowDelta: true, AllowStore: false})
	}
}
