package otelcli

import (
	"log"
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate completion script",
	Long: `To load completions:

Bash:

  $ source <(otel-cli completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ otel-cli completion bash > /etc/bash_completion.d/otel-cli
  # macOS:
  $ otel-cli completion bash > /usr/local/etc/bash_completion.d/otel-cli

Zsh:

  # If shell completion is not already enabled in your environment,
  # you will need to enable it.  You can execute the following once:

  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ otel-cli completion zsh > "${fpath[1]}/_otel-cli"

  # You will need to start a new shell for this setup to take effect.

fish:

  $ otel-cli completion fish | source

  # To load completions for each session, execute once:
  $ otel-cli completion fish > ~/.config/fish/completions/otel-cli.fish

PowerShell:

  PS> otel-cli completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> otel-cli completion powershell > otel-cli.ps1
  # and source this file from your PowerShell profile.
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.ExactValidArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		switch args[0] {
		case "bash":
			err := cmd.Root().GenBashCompletion(os.Stdout)
			if err != nil {
				log.Fatalf("failed to write completion to stdout")
			}
		case "zsh":
			err := cmd.Root().GenZshCompletion(os.Stdout)
			if err != nil {
				log.Fatalf("failed to write completion to stdout")
			}
		case "fish":
			err := cmd.Root().GenFishCompletion(os.Stdout, true)
			if err != nil {
				log.Fatalf("failed to write completion to stdout")
			}
		case "powershell":
			err := cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
			if err != nil {
				log.Fatalf("failed to write completion to stdout")
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
