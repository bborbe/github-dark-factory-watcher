// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package vaultmap resolves the vault slug a CreateTaskCommand must target
// from the GitHub repo the PR touches.
//
// The watcher watches one GitHub scope whose repos belong to different vaults,
// so a single global target vault cannot be correct: a command carrying no
// vault is claimed by the controller whose vault matches the hard-coded legacy
// default, and every other controller skips it as a cross-vault mismatch. A
// per-repo mapping lets one watcher serve all of them.
package vaultmap

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6@v6.12.2 -generate
