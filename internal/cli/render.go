package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/binding"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/pipelineconfig"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/render"
)

func newRenderCmd() *cobra.Command {
	var (
		quiet     bool
		values    []string
		namespace string
	)

	cmd := &cobra.Command{
		Use:   "render <release>",
		Short: "Render a release's chart locally, with placeholder platform values",
		Long: `Render a release's chart to stdout, using the native helm binary.

The chart comes from the release's entry in ` + "`.asgard-pipeline.yaml`" + `, and the
reserved ` + "`.Values.asgard.*`" + ` block is supplied with placeholders so a chart that
reads it renders rather than failing on a missing key.

    asgard-cli render internal-dev
    asgard-cli render internal-dev > .out/rendered.yaml
    asgard-cli render internal-dev | asgard-cli verify --rendered -

What this renders is not what will deploy. A run renders on the platform,
with the release's real values and real ids, and then checks every resulting CR
against the cluster's own CRDs with a server-side dry run. This renders with
placeholders and checks nothing. Use it for quick checks that do not need a
push - does the template compile, does it produce the objects I meant. The
authoritative answer is always the plan report:

    asgard-cli pipeline runs watch --release <name> --ref <tag>

The values a run would take from the platform are not fetched. Coercing a stored
string to the type its declaration gives it is the platform's rule, and this
command does not keep a copy of it. Use
-f to supply them by hand when a template needs them to render at all.

Everything except the manifests goes to stderr, so it pipes.

It needs helm on PATH and nothing else; ` + "`asgard-cli doctor`" + ` says whether it is
there.

It renders only; there is no install path. A Syncer pins its revision to
the chart's appVersion and only a run stamps a real ref in, so a local helm
upgrade would write the placeholder as a git ref that does not exist and the
Syncer would fail to clone on every run afterwards.

This is the ` + "`render`" + ` step of ` + "`asgard-cli gate`" + `, which renders every release
and then checks what came out. Run this one alone when you want the manifests
themselves rather than a verdict on them.`,
		// `render <project> <env>` was the old form. It is gone with the
		// per-environment values files, and cobra's own arity message would say
		// only "accepts 1 arg(s)" - which does not tell somebody typing the old
		// form what replaced it.
		Args: func(_ *cobra.Command, args []string) error {
			switch {
			case len(args) == 2 && (args[1] == "dev" || args[1] == "prod"):
				return fmt.Errorf(
					"render takes a release, not a project and an environment: `asgard-cli render <release>`.\n"+
						"Where a chart deploys is a release in %s now, and one chart can have several.\n"+
						"`asgard-cli pipeline releases` lists the ones the platform has.", pipelineconfig.FileName)
			case len(args) != 1:
				return fmt.Errorf("render takes exactly one release name")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			release := args[0]

			root, err := repoRoot()
			if err != nil {
				return err
			}

			errOut := cmd.ErrOrStderr()
			res, err := render.Run(cmd.Context(), render.Options{
				Root:        root,
				Release:     release,
				ValuesFiles: values,
				Namespace:   namespace,
			}, cmd.OutOrStdout(), errOut)
			if err != nil {
				return err
			}

			// The summary goes to stderr so that stdout stays exactly the
			// manifests, which is what makes the pipe forms above work.
			if !quiet {
				fmt.Fprintf(errOut, "rendered %s as helm release %s in namespace %s\n",
					release, res.Release, res.Namespace)
				fmt.Fprintf(errOut, "the asgard values are placeholders; the plan report is what deploys\n")
			}
			return nil
		},
	}

	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "do not write the summary line to stderr")
	cmd.Flags().StringArrayVarP(&values, "values", "f", nil,
		"an extra values file, repeatable and applied in order; the reserved asgard block still wins")
	cmd.Flags().StringVar(&namespace, "namespace", "",
		"namespace to render against; defaults to a placeholder, since the real one comes from the release's project")

	return cmd
}

// repoRoot is the directory the declaration lives in, which is what a local
// render resolves chart paths against.
//
// It is the working directory rather than a configured root: the declaration's
// chart paths are relative to the repository, and finding the declaration is
// how the repository is found.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	declPath, _, err := binding.Locate(dir)
	if err != nil {
		return "", err
	}
	if declPath == "" {
		return "", fmt.Errorf("no %s at or above %s, so there is no declaration to render from",
			pipelineconfig.FileName, dir)
	}
	return filepath.Dir(declPath), nil
}
