package main

import (
	"fmt"
	"os"

	"github.com/alibaba/open-code-review/internal/viewer"
	"github.com/spf13/cobra"
)

type viewerOptions struct {
	addr       string
	reviewsDir string
}

var viewerOpts viewerOptions

var viewerCmd = &cobra.Command{
	Use:     "viewer [flags]",
	Aliases: []string{"v"},
	Short:   "Start the WebUI session viewer",
	Long:    "Session history WebUI viewer.",
	Args:    cobra.NoArgs,
	Example: `  ocr viewer                     # start on default port
  ocr viewer --addr :3000        # bind to all interfaces on port 3000
  ocr viewer --reviews-dir /path/to/reviews`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if viewerOpts.reviewsDir == "" {
			viewerOpts.reviewsDir = os.Getenv("OCR_REVIEWS_DIR")
		}
		fmt.Printf("Open Code Review Viewer starting on http://%s\n", viewer.DisplayAddr(viewerOpts.addr))
		return viewer.StartServerWithOptions(viewer.ServerOptions{
			Addr:       viewerOpts.addr,
			ReviewsDir: viewerOpts.reviewsDir,
		})
	},
}

func init() {
	viewerCmd.Flags().StringVar(&viewerOpts.addr, "addr", "localhost:5483", "listen address")
	viewerCmd.Flags().StringVar(&viewerOpts.reviewsDir, "reviews-dir", "", "root directory for review result JSON files (env: OCR_REVIEWS_DIR)")
}
