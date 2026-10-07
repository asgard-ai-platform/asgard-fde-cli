package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/auth"
)

// `asgard-cli profile` names a platform this binary does not have compiled in.
//
// **The hosted platform needs none of this.** A customer who never runs these
// commands has no file, and every command reaches the hosted platform - which
// is the shape a tool for customers should have. What these are for is the two
// cases the binary cannot know: an on-prem installation, and a developer
// pointed at something running locally.
//
// **There is deliberately no `profile use`.** Recording which profile is
// current is the one thing that came out of the retired config.json and is not
// going back in: a preference stored on one machine is invisible there and
// absent everywhere else, so two people running the same command in the same
// checkout got different answers. Which profile is --profile, ASGARD_PROFILE,
// or `default`, and all three are visible at the moment they apply.
func newProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Name a platform this binary does not have built in",
		Long: `Name a platform this binary does not have built in.

    asgard-cli profile list              what is configured, and what applies now
    asgard-cli profile show [name]       the values, and where each came from
    asgard-cli profile set <name> ...    write one
    asgard-cli profile remove <name>     forget one

If you use the hosted Asgard platform, you need none of this. With no file
at all, every command reaches it; that is what ` + "`" + auth.DefaultProfileName + "`" + ` means, and it is
why it is the default. These commands exist for the two cases this binary
cannot know about: an on-prem installation, and a stack running locally.

A profile holds these values, and each falls back on its own:

    --platform-api    where the Asgard Platform API is
    --issuer          the Casdoor that issues tokens for it
    --client-id       the application this CLI presents itself as

Set one and the rest stay the hosted platform's, which is right for a local
Platform API against a real Casdoor and wrong for an on-prem installation - so
` + "`profile show`" + ` prints where every value came from, and says so when the API and
the identity provider disagree about where they are from.

Which profile applies is ` + "`--profile`" + `, then ` + "`" + auth.EnvProfile + "`" + `, then ` + "`" + auth.DefaultProfileName + "`" + `.
Nothing records a current profile. A preference stored on one machine is
invisible there and absent on every other.

The file is ` + "`profiles.json`" + `, beside the credentials. It holds no secret - a
client id is disclosed to the browser on every sign-in - so it can be handed to
a colleague setting up the same installation. Credentials are a separate file
and are not.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newProfileListCmd(), newProfileShowCmd(), newProfileSetCmd(), newProfileRemoveCmd())
	return cmd
}

func newProfileListCmd() *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the configured profiles, and say which one applies now",
		Long: `List the profiles ` + "`profiles.json`" + ` holds, and which one applies right now.

An empty list is the ordinary state of somebody using the hosted platform, not
something to fix. It writes nothing.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := checkFormat(format); err != nil {
				return err
			}
			file, err := auth.LoadProfiles()
			if err != nil {
				return err
			}
			path, _ := auth.ProfilesPath()
			current := auth.DefaultProfileName
			if v := os.Getenv(auth.EnvProfile); v != "" {
				current = v
			}

			out := cmd.OutOrStdout()
			if format == formatJSON {
				rows := make([]map[string]any, 0, len(file.Names()))
				for _, n := range file.Names() {
					p := file.Profiles[n]
					rows = append(rows, map[string]any{
						"name": n, "issuer": p.Issuer, "client_id": p.ClientID,
						"platform_api": p.PlatformAPI, "current": n == current,
					})
				}
				return writeJSON(out, map[string]any{"path": path, "current": current, "profiles": rows})
			}

			names := file.Names()
			if len(names) == 0 {
				fmt.Fprintf(out, "No profiles configured, so every command reaches the hosted platform.\n")
				fmt.Fprintf(out, "That is the ordinary state, and %q is what it is called.\n\n", auth.DefaultProfileName)
				fmt.Fprintf(out, "An on-prem installation, or a stack running locally, is named here:\n\n")
				fmt.Fprintf(out, "    asgard-cli profile set <name> --platform-api <url> --issuer <url> --client-id <id>\n")
				return nil
			}
			for _, n := range names {
				mark := "  "
				if n == current {
					mark = "* "
				}
				p := file.Profiles[n]
				api := p.PlatformAPI
				if api == "" {
					api = "(the hosted platform)"
				}
				fmt.Fprintf(out, "%s%-20s %s\n", mark, n, api)
			}
			if _, ok := file.Profiles[current]; !ok {
				fmt.Fprintf(out, "\n%q applies now and is not in the file, so it is the hosted platform.\n", current)
			}
			fmt.Fprintf(out, "\n%s\n", path)
			return nil
		},
	}
	cmd.Flags().StringVar(&format, formatFlag, formatText, formatUsage)
	return cmd
}

func newProfileShowCmd() *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:         "show [name]",
		Annotations: touchesNoNetwork(),
		Short:       "Report a profile's values, and where each came from",
		Long: `Report the values a profile resolves to, and where each one came from.

    asgard-cli profile show              whichever applies now
    asgard-cli profile show onprem-dev   a particular one

The provenance matters most. A profile that sets one value and inherits the
rest looks like a complete one, and the failure it causes shows up as a
permission error rather than as a mismatch.

It writes nothing and reaches no network.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkFormat(format); err != nil {
				return err
			}
			want := ""
			if len(args) == 1 {
				want = args[0]
			}
			r, err := auth.ResolveWithOrigin(want)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if format == formatJSON {
				return writeJSON(out, map[string]any{
					"name": r.Name, "configured": r.Exists,
					"platform_api": r.PlatformAPI, "platform_api_from": string(r.APIFrom),
					"issuer": r.Issuer, "issuer_from": string(r.IssuerFrom),
					"client_id": r.ClientID, "client_id_from": string(r.ClientIDFrom),
					"warning": r.Warning(),
				})
			}

			fmt.Fprintf(out, "%-14s %s\n", "profile", r.Name)
			fmt.Fprintf(out, "%-14s %-46s %s\n", "platform api", r.PlatformAPI, r.APIFrom)
			fmt.Fprintf(out, "%-14s %-46s %s\n", "issuer", r.Issuer, r.IssuerFrom)
			fmt.Fprintf(out, "%-14s %-46s %s\n", "client id", r.ClientID, r.ClientIDFrom)
			// An empty Console is printed rather than skipped: it is the one
			// field a profile can legitimately have none of, and a row that
			// disappears reads as a row nobody needed.
			shown, from := r.Console, fmt.Sprint(r.ConsoleFrom)
			if shown == "" {
				shown, from = "-", "not recorded; --console <url> sets it"
			}
			fmt.Fprintf(out, "%-14s %-46s %s\n", "console", shown, from)
			if w := r.Warning(); w != "" {
				fmt.Fprintf(out, "\nWARNING: %s\n", w)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, formatFlag, formatText, formatUsage)
	return cmd
}

func newProfileSetCmd() *cobra.Command {
	var issuer, clientID, platformAPI, console string

	cmd := &cobra.Command{
		Use:   "set <name>",
		Short: "Write a profile, creating the file if this is the first one",
		Long: `Write a profile.

    asgard-cli profile set onprem --platform-api https://asgard.acme.internal \
        --issuer https://iam.acme.internal --client-id abc123

This is the only command that creates the file. Nothing else writes it as a
side effect.

Only the flags given are changed, so a value can be corrected without restating
the others. Passing an empty string clears one, which makes it fall back to
the hosted platform again.

--console is where a person opens this installation, and nothing derives it.
The Console and the API are different hosts - platform.asgard-ai.com and
platform-api.asgard-ai.com on the hosted one - and removing "-api" from one to
get the other is not a rule other installations follow. So a profile
that names its own API has no Console until this records one, and the commands
that would send somebody to a page say so instead of guessing. It is also the
one field that does not fall back to the hosted value on its own: inheriting it
would point at another organisation's console.

An on-prem installation sets all three. Its API and the Casdoor that issues
tokens for it are the same installation, and a token from one is not accepted by
the other - so setting the API alone leaves you signing in against the hosted
Casdoor and presenting that token to somebody else's server. The command says so
when the result is mixed; it does not refuse, because a local Platform API
against a real Casdoor is a legitimate way to develop.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("a profile needs a name")
			}
			file, err := auth.LoadProfiles()
			if err != nil {
				return err
			}
			p := file.Profiles[name]
			p.Name = name
			// Only what was passed, so correcting one value does not require
			// restating the other two.
			if cmd.Flags().Changed("issuer") {
				p.Issuer = strings.TrimRight(issuer, "/")
			}
			if cmd.Flags().Changed("client-id") {
				p.ClientID = clientID
			}
			if cmd.Flags().Changed("platform-api") {
				p.PlatformAPI = strings.TrimRight(platformAPI, "/")
			}
			if cmd.Flags().Changed("console") {
				p.Console = strings.TrimRight(console, "/")
			}
			if p.Issuer == "" && p.ClientID == "" && p.PlatformAPI == "" && p.Console == "" && name != auth.DefaultProfileName {
				return fmt.Errorf("profile %q would set none of those values, which is the hosted platform.\n"+
					"That is what %q already is - use it, or give this one something to change:\n\n"+
					"    asgard-cli profile set %s --platform-api <url> --issuer <url> --client-id <id>",
					name, auth.DefaultProfileName, name)
			}
			file.Profiles[name] = p
			if err := auth.SaveProfiles(file); err != nil {
				return err
			}

			path, _ := auth.ProfilesPath()
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Wrote profile %q to %s.\n\n", name, path)

			r, err := auth.ResolveWithOrigin(name)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "%-14s %-46s %s\n", "platform api", r.PlatformAPI, r.APIFrom)
			fmt.Fprintf(out, "%-14s %-46s %s\n", "issuer", r.Issuer, r.IssuerFrom)
			fmt.Fprintf(out, "%-14s %-46s %s\n", "client id", r.ClientID, r.ClientIDFrom)
			if w := r.Warning(); w != "" {
				fmt.Fprintf(out, "\nWARNING: %s\n", w)
			}
			fmt.Fprintf(out, "\nUse it with --profile %s, or export %s=%s.\n", name, auth.EnvProfile, name)
			fmt.Fprintf(out, "Signing in is per profile: asgard-cli login --profile %s\n", name)
			return nil
		},
	}
	cmd.Flags().StringVar(&platformAPI, "platform-api", "", "the Asgard Platform API's base URL")
	cmd.Flags().StringVar(&issuer, "issuer", "", "the Casdoor base URL that issues tokens for it")
	cmd.Flags().StringVar(&clientID, "client-id", "", "the Casdoor application id this CLI presents (not a secret)")
	cmd.Flags().StringVar(&console, "console", "",
		"where a PERSON opens this installation, which is a different host from the API and is not derived from it")
	return cmd
}

func newProfileRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Forget a profile",
		Long: `Forget a profile.

The credential stored for it is left alone. They are separate files on purpose,
and a profile removed by mistake should not take a session with it; ` + "`asgard-cli logout --profile <name>`" + ` is how a session goes.

Removing a name that is not there is not an error - running this twice reports
the same thing both times.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			file, err := auth.LoadProfiles()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if _, ok := file.Profiles[name]; !ok {
				fmt.Fprintf(out, "No profile named %q.\n", name)
				return nil
			}
			delete(file.Profiles, name)
			if err := auth.SaveProfiles(file); err != nil {
				return err
			}
			fmt.Fprintf(out, "Removed profile %q. Its stored session, if any, is untouched:\n\n    asgard-cli logout --profile %s\n", name, name)
			return nil
		},
	}
	return cmd
}
