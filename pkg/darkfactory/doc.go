// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package darkfactory holds the network-backed candidate evaluator that
// decides whether an open draft PR carries an approved-not-completed
// dark-factory spec in its diff and therefore warrants a
// dark-factory-implement task.
package darkfactory

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6@v6.12.2 -generate
