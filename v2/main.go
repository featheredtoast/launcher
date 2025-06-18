package main

import (
	"context"
	"fmt"
	"github.com/alecthomas/kong"
	"github.com/discourse/launcher/v2/utils"
	"github.com/posener/complete"
	"github.com/willabides/kongplete"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

type Cli struct {
	Version      kong.VersionFlag   `help:"Show version."`
	ConfDir      string             `default:"./containers" hidden:"" help:"Discourse pups config directory." predictor:"dir"`
	TemplatesDir string             `default:"." hidden:"" help:"Home project directory containing a templates/ directory which in turn contains pups yaml templates." predictor:"dir"`
	BuildDir     string             `default:"" hidden:"" help:"Temporary build directory for building images." predictor:"dir"`
	Namespace    string             `default:"local_discourse" env:"DISCOURSE_NAMESPACE" help:"image namespace."`
	BuildCmd     DockerBuildCmd     `cmd:"" name:"build" help:"Build a base image. This command does not need a running database. Saves resulting container."`
	ConfigureCmd DockerConfigureCmd `cmd:"" name:"configure" help:"Configure and save an image with all dependencies and environment baked in. Updates themes and precompiles all assets. Saves resulting container."`
	MigrateCmd   DockerMigrateCmd   `cmd:"" name:"migrate" help:"Run migration tasks for a site. Running container is temporary and is not saved."`
	BootstrapCmd DockerBootstrapCmd `cmd:"" name:"bootstrap" help:"Builds, migrates, and configures an image. Resulting image is a fully built and configured Discourse image."`

	DestroyCmd DestroyCmd `cmd:"" name:"destroy" aliases:"down,rm" help:"Shutdown and destroy container."`
	LogsCmd    LogsCmd    `cmd:"" name:"logs" help:"Print logs for container."`
	CleanupCmd CleanupCmd `cmd:"" name:"cleanup" help:"Cleanup unused containers."`
	EnterCmd   EnterCmd   `cmd:"" name:"enter" help:"Connects to a shell running in the container."`
	RunCmd     RunCmd     `cmd:"" name:"run" help:"Runs the specified command in context of a docker container."`
	StartCmd   StartCmd   `cmd:"" name:"start" aliases:"up" help:"Starts container."`
	StopCmd    StopCmd    `cmd:"" name:"stop" help:"Stops container."`
	RestartCmd RestartCmd `cmd:"" name:"restart" help:"Stops then starts container."`
	RebuildCmd RebuildCmd `cmd:"" name:"rebuild" help:"Builds new image, then destroys old container, and starts new container."`

	InstallCompletions kongplete.InstallCompletions `cmd:"" aliases:"sh" help:"Print shell autocompletions. Add output to dotfiles, or 'source <(./launcher sh)'."`
}

func main() {
	cli := Cli{}
	runCtx, cancel := context.WithCancel(context.Background())

	// pre parse to get config dir for prediction of conf dir
	confFiles := utils.FindConfigNames()

	parser := kong.Must(&cli, kong.UsageOnError(), kong.Vars{"version": utils.Version})

	// Run kongplete.Complete to handle completion requests
	kongplete.Complete(parser,
		kongplete.WithPredictor("config", complete.PredictSet(confFiles...)),
		kongplete.WithPredictor("file", complete.PredictFiles("*")),
		kongplete.WithPredictor("dir", complete.PredictDirs("*")),
	)

	ctx, err := parser.Parse(os.Args[1:])
	parser.FatalIfErrorf(err)

	defer cancel()
	ctx.BindTo(runCtx, (*context.Context)(nil))
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM)
	signal.Notify(sigChan, syscall.SIGINT)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-sigChan:
			fmt.Fprintln(utils.Out, "Command interrupted") //nolint:errcheck
			cancel()
		case <-done:
		}
	}()
	err = ctx.Run()
	if err == nil {
		return
	}
	if exiterr, ok := err.(*exec.ExitError); ok {
		// Magic exit code that indicates a retry
		if exiterr.ExitCode() == 77 {
			os.Exit(77)
		} else if runCtx.Err() != nil {
			fmt.Fprintln(utils.Out, "Aborted with exit code", exiterr.ExitCode()) //nolint:errcheck
		} else {
			ctx.Fatalf(
				"run failed with exit code %v\n"+
					"** FAILED TO BOOTSTRAP ** please scroll up and look for earlier error messages, there may be more than one.\n"+
					"./discourse-doctor may help diagnose the problem.", exiterr.ExitCode())
		}
	} else {
		ctx.FatalIfErrorf(err)
	}
}
