package app

import (
	"context"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zerosonesfun/cs3-cli/internal/api"
	"github.com/zerosonesfun/cs3-cli/internal/config"
	"github.com/zerosonesfun/cs3-cli/internal/ui"
)

func cmdBits() *cobra.Command {
	var months bool
	var before string
	cmd := &cobra.Command{
		Use:   "bits [@username]",
		Short: "Show Bits for a profile (default: you)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			username := ""
			if len(args) > 0 {
				username = args[0]
			}
			return showBits(cmd.Context(), username, months, before)
		},
	}
	cmd.Flags().BoolVar(&months, "months", false, "Show two UTC months at a time")
	cmd.Flags().StringVar(&before, "before", "", "With --months: end month YYYY-MM (newer month of the pair)")
	return cmd
}

func showBits(ctx context.Context, username string, months bool, before string) error {
	c, err := client(true)
	if err != nil {
		return err
	}
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	if username == "" {
		cfg, _ := config.Load()
		username = strings.TrimSpace(cfg.Username)
		if username == "" {
			u, err := c.Me(ctx)
			if err != nil {
				return formatErr(err)
			}
			username = u.Username
			cfg.Username = username
			_ = config.Save(cfg)
		}
	}
	if months {
		return showBitsMonths(ctx, c, username, before)
	}
	all, err := c.Bits(ctx, username)
	if err != nil {
		return formatErr(err)
	}
	ui.Printf("@%s · %d bits\n", username, all.Count)
	if all.Count < 1 {
		ui.Println("No bits yet.")
		return nil
	}
	printBitCells(all.Cells)
	if all.Hidden > 0 {
		ui.Printf("+%d older\n", all.Hidden)
	}
	return nil
}

func showBitsMonths(ctx context.Context, c *api.Client, username, before string) error {
	cursor := strings.TrimSpace(before)
	for {
		pair, err := c.BitsMonths(ctx, username, cursor)
		if err != nil {
			return formatErr(err)
		}
		ui.Printf("@%s · %d bits (by month)\n", username, pair.Count)
		if len(pair.Months) == 0 {
			ui.Println("No months.")
			return nil
		}
		for _, m := range pair.Months {
			ui.Printf("\n%s · %d\n", m.Label, m.Earned)
			if m.Earned < 1 {
				ui.Println("  —")
				continue
			}
			printBitCells(m.Cells)
			if m.Hidden > 0 {
				ui.Printf("  +%d more\n", m.Hidden)
			}
		}
		ui.Println()
		if !pair.HasOlder {
			return nil
		}
		for {
			choice, err := ui.ReadLine("o = older, Enter = done: ")
			if err != nil {
				return err
			}
			choice = strings.TrimSpace(strings.ToLower(choice))
			if choice == "" || isBackCmd(choice) {
				return nil
			}
			if choice == "o" || choice == "older" {
				next := strings.TrimSpace(pair.OlderCursor)
				if next == "" || next == cursor {
					return nil
				}
				cursor = next
				break
			}
			ui.Println("Type o for older, or Enter to finish.")
		}
	}
}

func printBitCells(cells []api.BitCell) {
	if len(cells) == 0 {
		return
	}
	const cols = 20
	var b strings.Builder
	for i, cell := range cells {
		if i > 0 && i%cols == 0 {
			b.WriteByte('\n')
		}
		ch := cell.Char
		if ch == "" {
			ch = "▦"
		}
		b.WriteString(ch)
	}
	ui.Println(b.String())
}
