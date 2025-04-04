package cmd

import (
	"context"
	_ "embed"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"gabe565.com/castsponsorskip/internal/config"
	"gabe565.com/castsponsorskip/internal/device"
	"gabe565.com/castsponsorskip/internal/youtube"
	"gabe565.com/utils/cobrax"
	"github.com/spf13/cobra"
)

//go:embed description.md
var long string

func New(opts ...cobrax.Option) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "castsponsorskip",
		Short:   "Skip sponsored YouTube segments on local Cast devices",
		Long:    long,
		PreRunE: preRun,
		RunE:    run,

		ValidArgsFunction: func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		DisableAutoGenTag: true,
		SilenceErrors:     true,
	}

	config.InitLog(cmd.ErrOrStderr(), slog.LevelInfo, config.FormatAuto)
	config.RegisterFlags(cmd)
	config.RegisterCompletions(cmd)

	for _, opt := range opts {
		opt(cmd)
	}

	return cmd
}

func preRun(cmd *cobra.Command, _ []string) error {
	conf, err := config.Load(cmd)
	if err != nil {
		return err
	}

	cmd.SetContext(config.NewContext(cmd.Context(), conf))
	return nil
}

func run(cmd *cobra.Command, _ []string) error {
	conf := config.FromContext(cmd.Context())

	slog.Info("CastSponsorSkip", "version", cobrax.GetVersion(cmd), "commit", cobrax.GetCommit(cmd))

	ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	defer cancel()

	if conf.YouTubeAPIKey != "" {
		if err := youtube.CreateService(ctx, conf.YouTubeAPIKey); err != nil {
			return err
		}
	}

	entries, err := device.BeginDiscover(ctx, conf)
	if err != nil {
		return err
	}

	var group sync.WaitGroup
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case entry := <-entries:
				group.Add(1)
				go func() {
					if d := device.NewDevice(conf, entry, device.WithContext(ctx)); d != nil {
						_ = d.BeginTick()
						_ = d.Close()
					}
					group.Done()
				}()
			}
		}
	}()

	<-ctx.Done()
	slog.Info("Gracefully closing connections... Press Ctrl+C again to force exit.")

	forceCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	go func() {
		group.Wait()
		cancel()
	}()
	forceCtx, cancel = signal.NotifyContext(forceCtx, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	defer cancel()
	<-forceCtx.Done()
	slog.Info("Exiting.")
	return nil
}
