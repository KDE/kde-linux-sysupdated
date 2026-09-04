// SPDX-License-Identifier: BSD-3-Clause
// SPDX-FileCopyrightText: 2025-2026 Harald Sitter <sitter@kde.org>

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"path/filepath"

	"github.com/folbricht/desync"
)

type UpdateSizer struct {
	version string

	Size uint64

	// This acts as a context for our cache. This allows us to detect when the context changed and force recalculation of the update size.
	// e.g. when the user updates -> doesn't reboot -> updates again (different context)
	Context []string

	seeds      []desync.Seed
	remoteSeed *HTTPSeed

	remoteIndex desync.Index

	preparedContext bool
}

func NewUpdateSizer(version string) *UpdateSizer {
	return &UpdateSizer{
		version:         version,
		Size:            0,
		Context:         []string{version},
		preparedContext: false,
	}
}

func (u *UpdateSizer) PrepareContext() []string {
	erofses, err := filepath.Glob(globalConfig.SeedGlob)
	if err != nil {
		panic(err)
	}

	erofsStore, err := desync.NewLocalIndexStore(globalConfig.LocalIndexStorePath)
	if err != nil {
		panic(err)
	}
	defer erofsStore.Close()

	for _, erofsPath := range erofses {
		u.Context = append(u.Context, erofsPath)

		erofs := filepath.Base(erofsPath)
		desync.Log.Warn("Using erofs as seed ", erofs)

		index, err := erofsStore.GetIndex(erofs + ".caibx")
		if err != nil {
			desync.Log.Warn("Failed to get index for ", erofs, ": ", err)
			continue
		}

		seed, err := NewIndexSeed("", erofsPath, index)
		if err != nil {
			desync.Log.Warn("Failed to create seed for ", erofs, ": ", err)
			continue
		}
		u.seeds = append(u.seeds, seed)
	}

	u.preparedContext = true
	return u.Context
}

func (u *UpdateSizer) prepareRemote() error {
	url, err := url.Parse(globalConfig.UpstreamURL)
	if err != nil {
		return fmt.Errorf("failed to parse URL: %w", err)
	}

	file := fmt.Sprintf(globalConfig.ArtifactPathPattern, u.version)
	resp, err := http.DefaultClient.Head(url.JoinPath(file + ".caibx").String())
	if err != nil {
		return fmt.Errorf("failed to perform HEAD request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upstream responded with %d", resp.StatusCode)
	}
	url = resp.Request.URL
	url.Path = filepath.Dir(url.Path) // Use the final redirected URL base path

	remoteIndexStore, err := desync.NewRemoteHTTPIndexStore(url, desync.StoreOptions{})
	if err != nil {
		return fmt.Errorf("failed to create remote index store: %w", err)
	}

	desync.Log.Warn("Using remote index store at ", file)
	u.remoteIndex, err = remoteIndexStore.GetIndex(file + ".caibx")
	if err != nil {
		return fmt.Errorf("failed to get remote index: %w", err)
	}

	u.remoteSeed = NewHTTPSeed(url.JoinPath(file), u.remoteIndex)
	return nil
}

func (u *UpdateSizer) Calculate(ctx context.Context) (uint64, error) {
	if !u.preparedContext {
		return 0, fmt.Errorf("context not prepared")
	}

	err := u.prepareRemote()
	if err != nil {
		return 0, fmt.Errorf("failed to prepare remote: %w", err)
	}

	log.Println("Calculating update size with ", len(u.seeds), "seeds")

	var store desync.Store = nil // We don't actually need a store here, the requests come from remote anyway.
	assembler, err := stream(ctx, u.remoteIndex, u.remoteSeed, store, u.seeds, AssembleOptions{})
	if err != nil {
		desync.Log.Error("Failed to create stream: ", err)
		panic(err)
	}

	size := uint64(0)
	for _, segment := range assembler.plan {
		if !segment.isFileSeed() {
			size += uint64(segment.source.Size())
		}
	}

	fmt.Println("Calculated update size: ", size)

	return size, nil
}
