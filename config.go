// SPDX-License-Identifier: BSD-3-Clause
// SPDX-FileCopyrightText: 2025-2026 Harald Sitter <sitter@kde.org>

package main

import (
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	// Enable the store detection system. Depending on the remote the content addressable store is used instead of range requests.
	EnableStore bool `yaml:"enable_store"`
	// Force the use of the store even when the detection heuristics suggest range requests would be better.
	ForceStore bool `yaml:"force_store"`
	// Upstream URL to use for range requests when the store is not used.
	UpstreamURL string `yaml:"upstream_url"`
	// URL to use for the content addressable store.
	StoreURL string `yaml:"store_url"`
	// Path to the local index store on disk.
	LocalIndexStorePath string `yaml:"local_index_store_path"`
	// Filename pattern for update artifacts relative to UpstreamURL.
	// Must contain exactly one %s placeholder for the version.
	ArtifactPathPattern string `yaml:"artifact_path_pattern"`
	// Glob pattern to match seed files.
	SeedGlob string `yaml:"seed_glob"`
}

const DefaultConfigPath = "/run/kde-linux-sysupdated/config.yaml"

func LoadConfig(path string) Config {
	cfg := Config{
		EnableStore: os.Getenv("KDE_LINUX_SYSUPDATED_ENABLE_STORE") == "1",
		ForceStore:  os.Getenv("KDE_LINUX_SYSUPDATED_FORCE_STORE") == "1",
	}
	data, err := os.ReadFile(path)
	if err != nil {
		log.Println("Failed to read config, using defaults", err)
		return cfg
	}
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		log.Println("Failed to unmarshal config, using defaults", err)
		return cfg
	}
	return cfg
}
