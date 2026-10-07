package cli

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/platform"
)

// Variable kinds, as the platform names them.
const (
	kindChartValue = "chart_value"
	kindSecret     = "secret"
	kindConfig     = "config"
)

func newPipelineVariablesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "variables",
		Short: "Read and write a release's values",
		Long: `Read and write the values a release deploys with.

The repository declares which keys exist; the platform holds their values. So a
chart in a customer's repository never contains a password, a hostname or an
environment id, and a value can be changed without a commit.

    asgard-cli pipeline variables list --release internal-dev
    asgard-cli pipeline variables set --release internal-dev bpmDB.host db.internal
    asgard-cli pipeline variables set --release internal-dev --kind secret sendgrid_api_key --from-file ./key.txt
    asgard-cli pipeline variables sync-declared --release internal-dev

Saving changes nothing on the cluster. A value reaches the cluster only when a
run applies it, which is why the release then reports that the platform holds
something the cluster does not.

Secrets are write-only. A secret's value never comes back out of the platform,
for any caller: the list reports whether one is set and how many lines it has,
which is enough to tell a truncated PEM from a whole one without showing it.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newVariablesListCmd(),
		newVariablesSetCmd(),
		newVariablesUnsetCmd(),
		newVariablesSyncDeclaredCmd(),
	)
	return cmd
}

// variableFlags are the release-scoped flags every subcommand shares.
type variableFlags struct {
	pipelineFlags
	release string
}

func (v *variableFlags) register(cmd *cobra.Command) {
	v.pipelineFlags.register(cmd, true)
	cmd.Flags().StringVar(&v.release, "release", "", "release name, as the declaration spells it (required)")
}

// resolve produces the release a variables command acts on.
func (v *variableFlags) resolve(cmd *cobra.Command) (*platformContext, *platform.Release, error) {
	if v.release == "" {
		return nil, nil, fmt.Errorf("--release is required; `asgard-cli pipeline releases` lists them")
	}
	pc, err := v.pipelineFlags.context(cmd)
	if err != nil {
		return nil, nil, err
	}
	p, err := resolvePipeline(cmd.Context(), pc, v.pipeline)
	if err != nil {
		return nil, nil, err
	}
	rel, err := resolveRelease(cmd.Context(), pc, p, v.release)
	if err != nil {
		return nil, nil, err
	}
	return pc, rel, nil
}

func newVariablesListCmd() *cobra.Command {
	var v variableFlags

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List a release's values and the keys still missing one",
		Long: `List a release's stored values, and the declared keys that have none.

Every row says whether the declaration still names it. A row marked Orphan is
one the platform holds and the declaration does not: it is not injected, the
plan warns about it, and the fix is either to declare it or to delete it.

The header states which config the declared / required / orphan answers came
from - the release's own most recent run, or the pipeline's snapshot when it has
never run. Read that line before trusting the column: it is what explains why a
key is marked Orphan.

A required key with no value fails the plan with ` + "`vars/required-missing`" + `, which
is why those are listed first and separately.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pc, rel, err := v.resolve(cmd)
			if err != nil {
				return err
			}
			list, err := pc.Client.GetVariables(cmd.Context(), rel.ReleaseId)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if v.format == formatJSON {
				return writeJSON(out, list)
			}

			if list.DeclarationSource != nil {
				fmt.Fprintf(out, "declarations %s\n\n", list.DeclarationSource.Describe())
			}

			var missingRequired []*platform.DeclaredKey
			for _, k := range list.MissingDeclaredKeys {
				if k.Required {
					missingRequired = append(missingRequired, k)
				}
			}
			if len(missingRequired) > 0 {
				fmt.Fprintf(out, "REQUIRED, NO VALUE - the plan fails with vars/required-missing until these are set\n")
				for _, k := range missingRequired {
					fmt.Fprintf(out, "  %-12s %s\n", k.Kind, k.Key)
				}
				fmt.Fprintln(out)
			}

			if len(list.Variables) == 0 {
				fmt.Fprintf(out, "No values stored.\n")
			}
			for _, row := range list.Variables {
				fmt.Fprintf(out, "%-12s %-40s %-28s %s\n",
					row.Kind, row.Key, variableValue(row), variableNote(row))
			}

			if n := len(list.MissingDeclaredKeys); n > 0 {
				fmt.Fprintf(out, "\n%d declared key(s) have no row yet. Create them all as empty rows:\n\n"+
					"    asgard-cli pipeline variables sync-declared --release %s\n", n, rel.Name)
			}
			return nil
		},
	}
	v.register(cmd)
	return cmd
}

// variableValue renders a value for a reader, without ever printing a secret.
func variableValue(row *platform.Variable) string {
	if row.Kind == kindSecret {
		switch {
		case !row.IsSet:
			return "(not set)"
		case row.LineCount > 1:
			return fmt.Sprintf("(set, %d lines)", row.LineCount)
		default:
			return "(set)"
		}
	}
	if !row.IsSet {
		return "(not set)"
	}
	// A multi-line value is shown as its first line plus a count, so a pasted
	// certificate does not take over the table.
	if row.LineCount > 1 {
		first, _, _ := strings.Cut(row.Value, "\n")
		return fmt.Sprintf("%s +%d lines", truncateValue(first), row.LineCount-1)
	}
	return truncateValue(row.Value)
}

func truncateValue(s string) string {
	if len(s) <= 26 {
		return s
	}
	return s[:23] + "..."
}

func variableNote(row *platform.Variable) string {
	switch {
	case !row.Declared:
		return "ORPHAN - not declared, not injected"
	case row.Required:
		return "required"
	default:
		return "optional"
	}
}

func newVariablesSetCmd() *cobra.Command {
	var (
		v           variableFlags
		kind        string
		fromFile    string
		random      bool
		description string
	)

	cmd := &cobra.Command{
		Use:   "set <key> [value]",
		Short: "Set one value",
		Long: `Set one value on the platform.

    asgard-cli pipeline variables set --release dev bpmDB.host db.internal
    asgard-cli pipeline variables set --release dev --kind config recipients a@x,b@y
    asgard-cli pipeline variables set --release dev --kind secret db_password --from-file ./pw
    printf '%s' "$PASSWORD" | asgard-cli pipeline variables set --release dev --kind secret db_password --from-file -
    asgard-cli pipeline variables set --release dev --kind secret asgard_resource_api_key --random

--kind is chart_value by default, and is what the declaration lists the key
under: chartValues, appSecret or appConfigMap.

A secret's value cannot be an argument. It has to come from --from-file, or from
standard input with --from-file -, because a value typed as an argument is in
the shell history and in the process list of every other user on the machine.
Reading from a file also keeps a PEM intact: a file is read verbatim, with its newlines
and its trailing newline, and neither is trimmed.

--random is for a secret nobody issues - a key whose value only has to be
unguessable, like asgard_resource_api_key. It generates 32 random bytes as 64
hex characters, with no newline, the same on every operating system, and never
prints them.

Saving changes only the platform's copy. A run is what sends it to the cluster.`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			switch kind {
			case kindChartValue, kindSecret, kindConfig:
			default:
				return fmt.Errorf("no kind %q; one of %s, %s, %s", kind, kindChartValue, kindSecret, kindConfig)
			}

			value, err := readVariableValue(cmd, args, kind, fromFile, random)
			if err != nil {
				return err
			}

			pc, rel, err := v.resolve(cmd)
			if err != nil {
				return err
			}
			actingOn(cmd, pc.Session)
			write := platform.VariableWrite{Kind: kind, Key: key, Value: value}
			if cmd.Flags().Changed("description") {
				write.Description = &description
			}
			rows, err := pc.Client.PutVariables(cmd.Context(), rel.ReleaseId, []platform.VariableWrite{write})
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if v.format == formatJSON {
				return writeJSON(out, rows)
			}
			// Never echo what was set: for a secret it would defeat the reason
			// it came from a file, and for the others the list command shows it.
			fmt.Fprintf(out, "Set %s %s on release %s.\n", kind, key, rel.Name)
			fmt.Fprintf(out, "Saved on the platform only. A run is what sends it to the cluster.\n")
			return nil
		},
	}
	v.register(cmd)
	cmd.Flags().StringVar(&kind, "kind", kindChartValue,
		"which list the declaration names this key under: chart_value, secret or config")
	cmd.Flags().StringVar(&fromFile, "from-file", "",
		"read the value from this file verbatim, or from standard input with -; required for a secret unless --random")
	cmd.Flags().BoolVar(&random, "random", false,
		"generate the value: 32 random bytes as hex, for a secret nobody issues; never printed")
	cmd.Flags().StringVar(&description, "description", "",
		"a note stored with the value; omitted leaves any existing one alone")
	return cmd
}

// readVariableValue takes the value from an argument or a file, and refuses the
// combinations that would be ambiguous or unsafe.
func readVariableValue(cmd *cobra.Command, args []string, kind, fromFile string, random bool) (string, error) {
	hasArg := len(args) == 2

	// **Generated here, not by a shell.** The obvious recipe pipes `openssl
	// rand` through `tr` into --from-file -, and neither exists on a stock
	// Windows - where PowerShell's pipe would also re-encode the text and add
	// a CRLF that nothing trims.
	if random {
		switch {
		case kind != kindSecret:
			return "", fmt.Errorf("--random is for --kind %s: a generated value nobody knows is only useful as a credential", kindSecret)
		case hasArg || fromFile != "":
			return "", fmt.Errorf("--random and a value both given; one of them is being ignored, so say which")
		}
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return "", fmt.Errorf("generate the value: %w", err)
		}
		return hex.EncodeToString(b), nil
	}

	switch {
	case hasArg && fromFile != "":
		return "", fmt.Errorf("a value and --from-file both given; one of them is being ignored, so say which")
	case kind == kindSecret && hasArg:
		return "", fmt.Errorf(
			"a secret's value cannot be an argument: it would be in the shell history and in the process\n" +
				"list of every other user on this machine. Use --from-file <path>, --from-file - to read\n" +
				"it from standard input, or --random to generate one")
	case !hasArg && fromFile == "":
		return "", fmt.Errorf("no value; pass one as an argument, --from-file <path>, or --random for a secret nobody issues")
	case hasArg:
		return args[1], nil
	}

	var (
		data []byte
		err  error
	)
	if fromFile == "-" {
		data, err = io.ReadAll(cmd.InOrStdin())
	} else {
		data, err = os.ReadFile(fromFile)
	}
	if err != nil {
		return "", fmt.Errorf("read the value: %w", err)
	}
	// Deliberately not trimmed. A PEM ends with a newline, and a value that
	// arrives without one is a value the cluster will reject in a way nobody
	// traces back to here.
	return string(data), nil
}

func newVariablesUnsetCmd() *cobra.Command {
	var (
		v    variableFlags
		kind string
	)

	cmd := &cobra.Command{
		Use:   "unset <key>",
		Short: "Remove one value",
		Long: `Remove one value from the platform.

Variables have no id of their own - (kind, key) is the identity - so --kind is
part of naming which one.

Removing a value the declaration still requires does not fail here; it fails the
next plan, with ` + "`vars/required-missing`" + `. Removing one the declaration does not
name is how an Orphan is cleaned up.

Like every other write here, this changes the platform only.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch kind {
			case kindChartValue, kindSecret, kindConfig:
			default:
				return fmt.Errorf("no kind %q; one of %s, %s, %s", kind, kindChartValue, kindSecret, kindConfig)
			}
			pc, rel, err := v.resolve(cmd)
			if err != nil {
				return err
			}
			actingOn(cmd, pc.Session)
			if err := pc.Client.DeleteVariable(cmd.Context(), rel.ReleaseId, kind, args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed %s %s from release %s.\n", kind, args[0], rel.Name)
			return nil
		},
	}
	v.register(cmd)
	cmd.Flags().StringVar(&kind, "kind", kindChartValue, "chart_value, secret or config")
	return cmd
}

func newVariablesSyncDeclaredCmd() *cobra.Command {
	var v variableFlags

	cmd := &cobra.Command{
		Use:   "sync-declared",
		Short: "Create an empty row for every declared key that has none",
		Long: `Create an empty row for every key the declaration names that the release does not
have yet, so they can be filled in one at a time.

It adds rows and never removes or overwrites one, so running it twice is the
same as running it once.

Which declaration it reads is the release's own: its most recent run's config,
or the pipeline's snapshot when it has never run. ` + "`variables list`" + ` states which.
Keys added from the wrong config will be marked Orphan.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pc, rel, err := v.resolve(cmd)
			if err != nil {
				return err
			}
			actingOn(cmd, pc.Session)
			before, err := pc.Client.GetVariables(cmd.Context(), rel.ReleaseId)
			if err != nil {
				return err
			}
			rows, err := pc.Client.AddDeclaredKeys(cmd.Context(), rel.ReleaseId)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if v.format == formatJSON {
				return writeJSON(out, rows)
			}
			added := len(rows) - len(before.Variables)
			if added <= 0 {
				fmt.Fprintf(out, "Every declared key already has a row.\n")
				return nil
			}
			fmt.Fprintf(out, "Added %d empty row(s) on release %s.\n\n"+
				"    asgard-cli pipeline variables list --release %s\n", added, rel.Name, rel.Name)
			return nil
		},
	}
	v.register(cmd)
	return cmd
}
