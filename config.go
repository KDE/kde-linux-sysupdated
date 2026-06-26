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
}

const DefaultConfigPath = "/run/kde-linux-sysupdated/config.yaml"

func LoadConfig(path string) Config {
	cfg := Config{
		EnableStore: os.Getenv("KDE_LINUX_SYSUPDATED_ENABLE_STORE") != "0",
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
