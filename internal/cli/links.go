package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/auth"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/binding"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/gitrepo"
)

// newLinksCmd prints the URLs of the systems this checkout is bound to.
//
// **The ids are already recorded and the URL shapes are fixed, so assembling a
// link by hand is a typo away from a dead one in front of a room.** Three ways
// of getting one wrong, all from one slide: a link dropped because the author
// guessed the audience could not open it, a URL spelled out beside a name that
// was already the link, and the site's root in place of the page being
// discussed. None of them is a formatting question - the deck renders
// identically - and nothing catches them but somebody clicking.
//
// **What it will not do is guess.** The Console's host is not derivable from
// the API's, and the path to a pipeline is written down nowhere, so those come
// out as a named absence rather than as a plausible URL.
func newLinksCmd() *cobra.Command {
	var profile string

	cmd := &cobra.Command{
		Use:         "links",
		Annotations: touchesNoNetwork(),
		Short:       "The URLs of the systems this checkout is bound to",
		Long: `Print the repository and platform URLs for this engagement, from the ids already recorded.

    asgard-cli links

An internal or partner deck is mostly links, and every id in one is already on
disk: ` + "`.asgard-cli.yaml`" + ` carries the workspace, the git remote carries the
repository. Links assembled by hand go wrong in ways that still render: the
site root instead of the page, the URL printed beside a name that is already
the link, or a link dropped on a guess about who can open it.

It prints only what it knows. A row it cannot build says so and names where
that is recorded, because a URL built from a guessed host resolves to nothing
and looks the same as one that works.

The Console is a different host from the API, and neither can be derived from
the other. This tool is configured with the API's, so the Console is known for the hosted
installation and unknown for any other - ` + "`asgard-cli profile show`" + ` says which
one is in effect. Nothing here reaches the network or needs a session.

It needs a checkout, because the question is about this engagement. Outside one
it exits non-zero and says so.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()

			bind, bindErr := binding.LoadFrom(dir)
			origin, originErr := gitrepo.OriginURL(cmd.Context(), dir)
			// **Nothing to answer with is an error; a half answer is not.** A
			// checkout mid-setup has one of the two and is the ordinary state
			// this is run in.
			if bindErr != nil && originErr != nil {
				why := "this is not a git repository"
				if errors.Is(originErr, gitrepo.ErrNoOrigin) {
					why = "this git repository has no `origin` remote"
				}
				return fmt.Errorf("nothing to build a link from: %s holds no binding, and %s.\n"+
					"  `links` answers a question about one engagement, so it has nothing to say outside one",
					binding.FileName, why)
			}

			var missing []string

			if originErr == nil {
				if name, ok := gitrepo.FullName(origin); ok {
					fmt.Fprintf(out, "repo       https://github.com/%s\n", name)
				} else {
					fmt.Fprintf(out, "repo       %s\n", origin)
				}
			} else if errors.Is(originErr, gitrepo.ErrNoOrigin) {
				missing = append(missing, "repo: this git repository has no `origin` remote to read")
			} else {
				missing = append(missing, "repo: this is not a git repository, so there is no remote to read")
			}

			console, known := consoleURL(profile)
			switch {
			case bindErr != nil || bind.Workspace == "":
				missing = append(missing, fmt.Sprintf(
					"workspace: %s records none. `asgard-cli workspace use <id>` writes it", binding.FileName))
			case !known:
				missing = append(missing, "workspace: this profile records no Console, and it is not derivable "+
					"from the API. `asgard-cli profile set <name> --console <url>` records it")
			default:
				fmt.Fprintf(out, "workspace  %s/workspace/%s/overview\n", console, bind.Workspace)
			}

			// **The other two are not absent, they are unanswered.** A pipeline
			// has an id recorded here and no console path written down
			// anywhere; a platform project has neither. Printing a guess for
			// either is the defect this command exists to remove.
			// **The shapes are known and the ids are not, which is a different
			// absence from the one this used to report.** wiki/console.md has
			// the paths; what neither the binding nor the declaration records
			// is a release id or a platform project id, and both are in the
			// path rather than optional.
			if bindErr == nil && bind.Pipeline != "" && known {
				missing = append(missing, fmt.Sprintf(
					"pipeline: id %s is recorded, and the page is per RELEASE - "+
						"%s/workspace/<ws>/pipelines/%s/releases/<releaseId> - which nothing here records",
					bind.Pipeline, console, bind.Pipeline))
			}
			missing = append(missing, "project: the page is /workspace/<ws>/project/<projectId>, and no platform "+
				"project id is recorded in this checkout - it appears in .asgard-pipeline.yaml only as a comment. "+
				"Every per-resource page hangs off that same prefix, so a Toolset, Skillset, Drive or Agent link "+
				"needs it too - wiki/console.md has the shapes")

			if len(missing) > 0 {
				fmt.Fprintln(out)
				for _, m := range missing {
					fmt.Fprintf(out, "  not printed, %s\n", m)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "which profile decides the Console's host; defaults to the one in effect")
	return cmd
}

// consoleURL resolves the profile only far enough to ask where its Console is.
// A profile that cannot be resolved is not an error here: the repository half
// of the answer does not depend on one.
func consoleURL(want string) (string, bool) {
	p, err := auth.ResolveProfile(want)
	if err != nil {
		return "", false
	}
	return p.ConsoleURL()
}
