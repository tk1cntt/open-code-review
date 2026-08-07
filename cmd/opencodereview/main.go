// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/telemetry"
)

func main() {
	llm.AppVersion = Version
	llm.InitEmbeddedLoader()

	// Graceful shutdown: SIGINT (Ctrl+C) triggers context cancellation so
	// workers drain, checkpoints flush, and session_end is written before
	// the process exits. The parent context is set on rootCmd so every
	// subcommand inherits it; review/scan/refactor use it in place of
	// context.Background().
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if telemetry.Init(ctx) {
		// Use context.Background() for the shutdown timeout: the signal-aware
		// ctx may already be cancelled by the time this defer runs, which would
		// race with final flush. A clean background context guarantees the
		// exporter gets its 5-second grace window regardless of the interrupt.
		defer telemetry.ShutdownWithTimeout(context.Background(), 5*time.Second)
	}

	rootCmd.SetContext(ctx)

	if err := rootCmd.Execute(); err != nil {
		if errors.Is(err, context.Canceled) || ctx.Err() != nil {
			fmt.Fprintf(os.Stderr, "\n[ocr] Interrupted by user (Ctrl+C). Checkpoint saved — you can resume with:\n")
			fmt.Fprintf(os.Stderr, "[ocr]   ocr session list    (to see all sessions)\n")
			fmt.Fprintf(os.Stderr, "[ocr]   ocr <review|scan|refactor> --resume <session-id>\n")
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
