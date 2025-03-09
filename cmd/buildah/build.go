package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/containers/buildah/imagebuildah"
	buildahcli "github.com/containers/buildah/pkg/cli"
	"github.com/containers/buildah/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func init() {
	buildDescription := `
  Builds an OCI image using instructions in one or more Containerfiles.

  If no arguments are specified, Buildah will use the current working directory
  as the build context and look for a Containerfile. The build fails if no
  Containerfile nor Dockerfile is present.`

	layerFlagsResults := buildahcli.LayerResults{}
	buildFlagResults := buildahcli.BudResults{}
	fromAndBudResults := buildahcli.FromAndBudResults{}
	userNSResults := buildahcli.UserNSResults{}
	namespaceResults := buildahcli.NameSpaceResults{}

	buildCommand := &cobra.Command{
		Use:     "build [CONTEXT]",
		Aliases: []string{"build-using-dockerfile", "bud"},
		Short:   "Build an image using instructions in a Containerfile",
		Long:    buildDescription,
		RunE: func(cmd *cobra.Command, args []string) error {
			br := buildahcli.BuildOptions{
				LayerResults:      &layerFlagsResults,
				BudResults:        &buildFlagResults,
				UserNSResults:     &userNSResults,
				FromAndBudResults: &fromAndBudResults,
				NameSpaceResults:  &namespaceResults,
			}
			return buildCmd(cmd, args, br)
		},
		Args: cobra.MaximumNArgs(1),
		Example: `buildah build
  buildah bud -f Containerfile.simple .
  buildah bud --volume /home/test:/myvol:ro,Z -t imageName .
  buildah bud -f Containerfile.simple -f Containerfile.notsosimple .`,
	}
	buildCommand.SetUsageTemplate(UsageTemplate())

	flags := buildCommand.Flags()
	flags.SetInterspersed(false)

	// build is a all common flags
	buildFlags := buildahcli.GetBudFlags(&buildFlagResults)
	buildFlags.StringVar(&buildFlagResults.Runtime, "runtime", util.Runtime(), "`path` to an alternate runtime. Use BUILDAH_RUNTIME environment variable to override.")

	layerFlags := buildahcli.GetLayerFlags(&layerFlagsResults)
	fromAndBudFlags, err := buildahcli.GetFromAndBudFlags(&fromAndBudResults, &userNSResults, &namespaceResults)
	if err != nil {
		logrus.Errorf("failed to setup From and Build flags: %v", err)
		os.Exit(1)
	}

	flags.AddFlagSet(&buildFlags)
	flags.AddFlagSet(&layerFlags)
	flags.AddFlagSet(&fromAndBudFlags)
	flags.SetNormalizeFunc(buildahcli.AliasFlags)

	rootCmd.AddCommand(buildCommand)
}

func buildCmd(c *cobra.Command, inputArgs []string, iopts buildahcli.BuildOptions) error {
	if iopts.EphemeralDataDir {
		if err := createEphemeralDataDir(c); err != nil {
			return fmt.Errorf("error creating ephemeral dir: %w", err)
		}
	}

	if c.Flag("logfile").Changed {
		logfile, err := os.OpenFile(iopts.Logfile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			return fmt.Errorf("opening log file: %w", err)
		}
		iopts.Logwriter = logfile
		defer iopts.Logwriter.Close()
	}

	options, containerfiles, removeAll, err := buildahcli.GenBuildOptions(c, inputArgs, iopts)
	if err != nil {
		return err
	}
	defer func() {
		for _, f := range removeAll {
			os.RemoveAll(f)
		}
	}()

	options.DefaultMountsFilePath = globalFlagResults.DefaultMountsFile

	store, err := getStore(c)
	if err != nil {
		return err
	}

	id, ref, err := imagebuildah.BuildDockerfiles(getContext(), store, options, containerfiles...)
	if err == nil && options.Manifest != "" {
		logrus.Debugf("manifest list id = %q, ref = %q", id, ref.String())
	}
	return err
}

func createEphemeralDataDir(c *cobra.Command) error {
	if c.Flag("root").Changed || c.Flag("runroot").Changed || c.Flag("blob-info-cache-dir").Changed {
		return fmt.Errorf("cannot request ephemeral data dir if root, runroot or blob-info-cache-dir flag is set")
	}
	td, err := os.MkdirTemp("", "bud")
	if err != nil {
		return fmt.Errorf("error creating ephemeral data directory: %w", err)
	}
	poststoreShutdownHandlers = append(poststoreShutdownHandlers, func() error {
		logrus.Debugf("cleaning up ephemeral directory: %s", td)
		return os.RemoveAll(td)
	})
	logrus.Debugf("created ephemeral directory: %s", td)
	if err := c.Flags().Set("root", filepath.Join(td, "root")); err != nil {
		return fmt.Errorf("error setting root: %w", err)
	}
	if err := c.Flags().Set("runroot", filepath.Join(td, "runroot")); err != nil {
		return fmt.Errorf("error setting runroot: %w", err)
	}
	if err := c.Flags().Set("blob-info-cache-dir", filepath.Join(td, "cache")); err != nil {
		return fmt.Errorf("error setting cache dir: %w", err)
	}
	return nil
}
