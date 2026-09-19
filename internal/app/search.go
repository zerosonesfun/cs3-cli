package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/zerosonesfun/cs3-cli/internal/ui"
	"github.com/spf13/cobra"
)

func cmdSearch() *cobra.Command {
	return &cobra.Command{
		Use:   "search [query...]",
		Short: "Search posts, people, and Clicks",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			q := strings.TrimSpace(strings.Join(args, " "))
			if q == "" {
				var err error
				q, err = ui.ReadLine("Query: ")
				if err != nil {
					return err
				}
				q = strings.TrimSpace(q)
			}
			if utf8.RuneCountInString(q) < 2 {
				return fmt.Errorf("query must be at least 2 characters")
			}
			return showSearch(cmd.Context(), q)
		},
	}
}

func showSearch(ctx context.Context, q string) error {
	c, err := client(true)
	if err != nil {
		return err
	}
	posts, users, clicks, err := c.Search(ctx, q)
	if err != nil {
		return formatErr(err)
	}

	ui.Println("Users")
	if len(users) == 0 {
		ui.Println("  None")
	} else {
		for _, u := range users {
			ui.Printf("  @%s\n", u.Username)
		}
	}
	ui.Println()

	ui.Println("Clicks")
	if len(clicks) == 0 {
		ui.Println("  None")
	} else {
		for _, cl := range clicks {
			line := fmt.Sprintf("  %s (/%s)", cl.Name, cl.Slug)
			if cl.IsMember {
				line += " · member"
			}
			ui.Println(line)
		}
	}
	ui.Println()

	ui.Println("Posts")
	if len(posts) == 0 {
		ui.Println("  None")
		return nil
	}
	printPosts(posts)

	for {
		choice, err := ui.ReadLine("Post # (Enter or b to skip): ")
		if err != nil || choice == "" || isBackCmd(choice) {
			return nil
		}
		n, err := strconv.Atoi(choice)
		if err != nil || n < 1 || n > len(posts) {
			ui.Printf("Pick a number between 1 and %d, or Enter to skip.\n", len(posts))
			continue
		}
		_, err = showPostThread(ctx, posts[n-1].ID)
		if err != nil {
			if isUnauthorized(err) {
				return err
			}
			ui.Printf("Error: %v\n", err)
			continue
		}
		return nil
	}
}
