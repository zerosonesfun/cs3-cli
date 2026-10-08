package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/zerosonesfun/cs3-cli/internal/api"
	"github.com/zerosonesfun/cs3-cli/internal/ui"
)

func cmdClick() *cobra.Command {
	click := &cobra.Command{
		Use:   "click",
		Short: "Search Clicks you belong to, or edit a description you own",
	}
	click.AddCommand(cmdClickSearch(), cmdClickDescription())
	return click
}

func cmdClickSearch() *cobra.Command {
	return &cobra.Command{
		Use:   "search [slug] [query...]",
		Short: "Search posts inside one Click",
		Long: `Search posts inside a single Click you are a member of.

Examples:
  cs3 click search night-writers coffee
  cs3 click search night-writers
  cs3 click search

If slug is omitted, pick from your Clicks. If query is omitted, you are prompted.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := ""
			queryParts := args
			if len(args) > 0 {
				slug = strings.TrimSpace(args[0])
				queryParts = args[1:]
			}
			q := strings.TrimSpace(strings.Join(queryParts, " "))
			return showClickSearch(cmd.Context(), slug, q)
		},
	}
}

func showClickSearch(ctx context.Context, slug, q string) error {
	c, err := client(true)
	if err != nil {
		return err
	}

	clickName := slug
	if slug == "" {
		clicks, err := c.MyClicks(ctx)
		if err != nil {
			return formatErr(err)
		}
		if len(clicks) == 0 {
			return fmt.Errorf("you are not in any Clicks")
		}
		for i, cl := range clicks {
			ui.Printf("%2d. %s (/%s)\n", i+1, cl.Name, cl.Slug)
		}
		pick, err := ui.ReadLine("Click #: ")
		if err != nil {
			return err
		}
		n, err := parsePick(pick, len(clicks))
		if err != nil {
			return err
		}
		slug = clicks[n-1].Slug
		clickName = clicks[n-1].Name
	} else {
		clicks, err := c.MyClicks(ctx)
		if err != nil {
			return formatErr(err)
		}
		for _, cl := range clicks {
			if cl.Slug == slug {
				clickName = cl.Name
				break
			}
		}
	}

	if q == "" {
		q, err = ui.ReadLine("Query: ")
		if err != nil {
			return err
		}
		q = strings.TrimSpace(q)
	}
	if utf8.RuneCountInString(q) < 2 {
		return fmt.Errorf("query must be at least 2 characters")
	}

	ui.Printf("\nSearching in Click: %s (/%s)\n\n", clickName, slug)

	var all []api.Post
	page := 1
	for {
		posts, hasMore, err := c.ClickSearch(ctx, slug, q, page)
		if err != nil {
			if apiErr, ok := err.(*api.APIError); ok && apiErr.Status == 403 {
				return fmt.Errorf("you must be a member of this Click")
			}
			return formatErr(err)
		}
		if page == 1 && len(posts) == 0 {
			ui.Printf("No posts matching %q in Click: %s.\n", q, clickName)
			return nil
		}
		all = append(all, posts...)
		if !hasMore {
			break
		}
		page++
		ok, err := ui.Confirm("Load more")
		if err != nil || !ok {
			break
		}
	}

	ui.Println("Posts")
	printPosts(all)

	for {
		choice, err := ui.ReadLine("Post # (Enter or b to skip): ")
		if err != nil || choice == "" || isBackCmd(choice) {
			return nil
		}
		n, err := parsePick(choice, len(all))
		if err != nil {
			ui.Printf("Pick a number between 1 and %d, or Enter to skip.\n", len(all))
			continue
		}
		_, err = showPostThread(ctx, all[n-1].ID)
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

func cmdClickDescription() *cobra.Command {
	var set bool
	cmd := &cobra.Command{
		Use:   "description [slug]",
		Short: "Show or replace a Click description",
		Long: `Print a Click description, including line breaks.

With --set, replace it. Type the new description and end with a line containing only .
An empty description clears it. Only the Click owner can replace it.

Examples:
  cs3 click description night-writers
  cs3 click description night-writers --set

If slug is omitted, pick from your Clicks.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := ""
			if len(args) > 0 {
				slug = strings.TrimSpace(args[0])
			}
			return runClickDescription(cmd.Context(), slug, set)
		},
	}
	cmd.Flags().BoolVar(&set, "set", false, "Replace the description")
	return cmd
}

func runClickDescription(ctx context.Context, slug string, set bool) error {
	c, err := client(true)
	if err != nil {
		return err
	}
	if slug == "" {
		picked, err := pickClickSlug(ctx, c)
		if err != nil {
			return err
		}
		slug = picked
	}
	if !set {
		click, _, _, _, err := c.Click(ctx, slug)
		if err != nil {
			return formatErr(err)
		}
		if click.Description == "" {
			ui.Println("No description.")
			return nil
		}
		ui.Println(click.Description)
		return nil
	}
	ui.Println("Description (end with a line containing only .). An empty description clears it.")
	body, err := ui.ReadMultiline(".")
	if err != nil {
		return err
	}
	click, err := c.UpdateClickDescription(ctx, slug, body)
	if err != nil {
		return formatErr(err)
	}
	if click.Description == "" {
		ui.Println("Description cleared.")
		return nil
	}
	ui.Println("Description saved.")
	return nil
}

func pickClickSlug(ctx context.Context, c *api.Client) (string, error) {
	clicks, err := c.MyClicks(ctx)
	if err != nil {
		return "", formatErr(err)
	}
	if len(clicks) == 0 {
		return "", fmt.Errorf("you are not in any Clicks")
	}
	for i, cl := range clicks {
		ui.Printf("%2d. %s (/%s)\n", i+1, cl.Name, cl.Slug)
	}
	pick, err := ui.ReadLine("Click #: ")
	if err != nil {
		return "", err
	}
	n, err := parsePick(pick, len(clicks))
	if err != nil {
		return "", err
	}
	return clicks[n-1].Slug, nil
}

func parsePick(s string, max int) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 1 || n > max {
		return 0, fmt.Errorf("pick a number between 1 and %d", max)
	}
	return n, nil
}
