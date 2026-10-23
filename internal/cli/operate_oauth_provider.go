package cli

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/platform"
)

func newOperateOAuthProviderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "oauth-provider",
		Short: "Open an OAuthProvider to members' own accounts, or close it",
		Long: `Open an OAuthProvider to members' own accounts, or close it.

    asgard-cli operate oauth-provider list --release <r>
    asgard-cli operate oauth-provider enable <name> --release <r>
    asgard-cli operate oauth-provider disable <name> --release <r>

An OAuthProvider whose chart gives it the label
asgard-ai.com/personal-account: eligible MAY be used with members' own
accounts: each member connects their own account in Connections, where they
talk to agents, and an agent then works as the member it is talking to - with
asgard-keyring in its sandbox. Whether members CAN, in a given project, is a
switch in Asgard Connector, not in the chart: the label says it is allowed,
the switch says it is on. A deploy never turns it on. This is how it is turned
on, after the deploy, by somebody allowed to decide it for the project.

The project's admins see and flip the same switch in Odin, Settings ›
Connection Points › the connection point › Individual Credentials. There is no
command that removes members' connections: that is Odin's, behind a
confirmation, because it cannot be undone.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newOperateOAuthProviderListCmd(), newOperateOAuthProviderSwitchCmd(true), newOperateOAuthProviderSwitchCmd(false))
	return cmd
}

func newOperateOAuthProviderListCmd() *cobra.Command {
	var f operateFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Which OAuthProviders members may connect their own account to, and which are open",
		Long: `Which OAuthProviders members may connect their own account to, and which are
open now.

    asgard-cli operate oauth-provider list --release internal-dev
    asgard-cli operate oauth-provider list --project <id|name|namespace>

With --release, the OAuthProviders that release deployed; with --project, all
of the project's. The columns:

    MEMBERS       open       members can connect their own account now
                  closed     it was open; their connections are kept
                  not open   it has never been opened
                  -          not allowed: no asgard-ai.com/personal-account
                             label, so only shared OAuthCredentials use it
                  unknown    Asgard Connector did not answer
    CONNECTED     members connected, and how many must reconnect
    DISPLAY NAME  what members see in Connections; - when there is none

Needs project-resource/read.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pc, scope, err := f.resolve(cmd)
			if err != nil {
				return err
			}
			all, err := pc.Client.ListOAuthProviders(cmd.Context(), scope.ProjectID)
			if err != nil {
				return err
			}
			providers := all
			if scope.live != nil {
				deployed := map[string]bool{}
				for _, o := range scope.live.Objects {
					if o.Kind == "OAuthProvider" {
						deployed[o.Name] = true
					}
				}
				providers = providers[:0:0]
				for _, p := range all {
					if deployed[p.ID] {
						providers = append(providers, p)
					}
				}
			}
			sort.Slice(providers, func(i, j int) bool { return providers[i].ID < providers[j].ID })
			out := cmd.OutOrStdout()
			if f.format == formatJSON {
				if providers == nil {
					providers = []platform.OAuthProvider{}
				}
				return writeJSON(out, providers)
			}
			if len(providers) == 0 {
				fmt.Fprintf(out, "%s has no OAuthProvider.\n", scope.Where)
				return nil
			}
			writeOAuthProviders(out, providers)
			return nil
		},
	}
	f.register(cmd)
	return cmd
}

func newOperateOAuthProviderSwitchCmd(open bool) *cobra.Command {
	var f operateFlags
	use, short, long := "disable <oauth-provider>", "Close an OAuthProvider to members' own accounts", `Close an OAuthProvider to members' own accounts.

    asgard-cli operate oauth-provider disable oap-google-drive --release internal-dev

At once: no new connections, no tokens handed out, no refreshes, and
Connections stops listing it. Members' connections are kept, so opening it
again needs nobody to reconnect. An access token an agent already holds works
until it expires, usually within the hour. Closing is always allowed.

Needs oauth-provider/put in the project (Project Admin).`
	if open {
		use, short, long = "enable <oauth-provider>", "Open an OAuthProvider to members' own accounts", `Open an OAuthProvider to members' own accounts.

    asgard-cli operate oauth-provider enable oap-google-drive --release internal-dev

Once it is open:

  - every member of this project sees it in Connections, under its display
    name, and can connect their own account (members who connected before can
    use theirs again at once);
  - from the next message on, an agent can act as the member it is talking to,
    with that member's account;
  - members sign in themselves, so the OAuth app has to accept their accounts
    (Google: internal, or published - not testing);
  - only this project: another project, another release's included, is opened
    on its own.

The chart has to have given the OAuthProvider the label
asgard-ai.com/personal-account: eligible and the annotation
asgard-ai.com/display-name; the platform refuses it otherwise, and says which.
asgard-ai.com/description and spec.endpoints.userInfoURL are recommended:
without them members do not know what connecting is for, or which account they
connected.

Needs oauth-provider/put in the project (Project Admin).`
	}
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Long:  long,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pc, scope, err := f.resolve(cmd)
			if err != nil {
				return err
			}
			name := args[0]
			if _, err := scope.object("OAuthProvider", name); err != nil {
				return err
			}
			actingOn(cmd, pc.Session)
			state, err := pc.Client.SetIndividualCredentials(cmd.Context(), scope.ProjectID, name, open)
			if err != nil {
				return explainSwitchRefusal(err, name)
			}
			out := cmd.OutOrStdout()
			if f.format == formatJSON {
				return writeJSON(out, state)
			}
			if open {
				fmt.Fprintf(out, "OAuthProvider %s is open to the members of %s: they see it in Connections and can connect their own account.\n", name, scope.Where)
			} else {
				fmt.Fprintf(out, "OAuthProvider %s is closed to the members of %s. Their connections are kept; tokens agents already hold work until they expire.\n", name, scope.Where)
			}
			fmt.Fprintf(out, "%d connected, %d to reconnect.\n", state.ConnectedCount, state.NeedsReauthCount)
			return nil
		},
	}
	f.register(cmd)
	return cmd
}

// explainSwitchRefusal turns the platform's refusal to open an OAuthProvider
// into what to change in the chart.
func explainSwitchRefusal(err error, name string) error {
	var apiErr *platform.APIError
	if !errors.As(err, &apiErr) {
		return err
	}
	switch apiErr.ReasonCode {
	case platform.ReasonConnectorProviderDisplayNameMissing:
		return fmt.Errorf("OAuthProvider %s has no display name, so it cannot be opened: members would see %q in Connections. "+
			"Add the annotation asgard-ai.com/display-name to it in the chart, deploy, and run this again", name, name)
	case platform.ReasonConnectorProviderNotEligible:
		return fmt.Errorf("OAuthProvider %s does not let members connect their own account. "+
			"Add the label asgard-ai.com/personal-account: eligible and the annotation asgard-ai.com/display-name to it in the chart, deploy, and run this again", name)
	}
	return err
}

// writeOAuthProviders prints the table. The display name goes last: it is
// often CJK, which tabwriter counts one column per rune although a terminal
// draws two, so any column after it would be pushed out of line.
func writeOAuthProviders(out io.Writer, providers []platform.OAuthProvider) {
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tMEMBERS\tCONNECTED\tDISPLAY NAME")
	for _, p := range providers {
		display := p.DisplayName
		if display == "" {
			display = "-"
		}
		members, connected := "-", "-"
		switch ic := p.IndividualCredentials; {
		case !p.PersonalAccountEligible:
		case ic == nil:
			members = "unknown"
		default:
			members = map[string]string{
				platform.IndividualCredentialsOpen:    "open",
				platform.IndividualCredentialsClosed:  "closed",
				platform.IndividualCredentialsNotOpen: "not open",
			}[ic.Status]
			connected = fmt.Sprintf("%d", ic.ConnectedCount)
			if ic.NeedsReauthCount > 0 {
				connected += fmt.Sprintf(" (%d to reconnect)", ic.NeedsReauthCount)
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", p.ID, members, connected, display)
	}
	_ = tw.Flush()
}
