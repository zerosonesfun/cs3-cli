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
		Short: "Search Clicks, join requests, or edit a description you own",
	}
	click.AddCommand(
		cmdClickSearch(),
		cmdClickDescription(),
		cmdClickRequest(),
		cmdClickCancelRequest(),
		cmdClickRequests(),
		cmdClickAllow(),
		cmdClickReject(),
		cmdClickAllowAll(),
	)
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

func cmdClickRequest() *cobra.Command {
	return &cobra.Command{
		Use:   "request [slug]",
		Short: "Request to join a Click",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := ""
			if len(args) > 0 {
				slug = strings.TrimSpace(args[0])
			}
			return runClickRequest(cmd.Context(), slug)
		},
	}
}

func cmdClickCancelRequest() *cobra.Command {
	return &cobra.Command{
		Use:   "cancel-request [slug]",
		Short: "Cancel your pending join request",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := ""
			if len(args) > 0 {
				slug = strings.TrimSpace(args[0])
			}
			return runClickCancelRequest(cmd.Context(), slug)
		},
	}
}

func cmdClickRequests() *cobra.Command {
	return &cobra.Command{
		Use:   "requests [slug]",
		Short: "List pending join requests (Click mods)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := ""
			if len(args) > 0 {
				slug = strings.TrimSpace(args[0])
			}
			return runClickRequests(cmd.Context(), slug)
		},
	}
}

func cmdClickAllow() *cobra.Command {
	return &cobra.Command{
		Use:   "allow <request-id>",
		Short: "Allow a pending join request (Click mod)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(strings.TrimSpace(args[0]))
			if err != nil || id < 1 {
				return fmt.Errorf("usage: cs3 click allow <request-id>")
			}
			return runClickAllow(cmd.Context(), id)
		},
	}
}

func cmdClickReject() *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "reject <request-id>",
		Short: "Reject a pending join request (Click mod)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(strings.TrimSpace(args[0]))
			if err != nil || id < 1 {
				return fmt.Errorf("usage: cs3 click reject <request-id>")
			}
			return runClickReject(cmd.Context(), id, reason)
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "Reject reason (required; prompted if omitted)")
	return cmd
}

func cmdClickAllowAll() *cobra.Command {
	return &cobra.Command{
		Use:   "allow-all [slug]",
		Short: "Approve all pending join requests for a Click (Click mod)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := ""
			if len(args) > 0 {
				slug = strings.TrimSpace(args[0])
			}
			return runClickAllowAll(cmd.Context(), slug)
		},
	}
}

func pickDirectoryClickSlug(ctx context.Context, c *api.Client) (string, error) {
	clicks, _, err := c.ListClicks(ctx, 1)
	if err != nil {
		return "", formatErr(err)
	}
	if len(clicks) == 0 {
		return "", fmt.Errorf("no Clicks found")
	}
	for i, cl := range clicks {
		status := ""
		if cl.IsMember {
			status = " (member)"
		} else if cl.IsKicked {
			status = " (invite required)"
		} else if cl.JoinPending {
			status = " (requested)"
		}
		ui.Printf("%2d. %s (/%s)%s\n", i+1, cl.Name, cl.Slug, status)
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

func runClickRequest(ctx context.Context, slug string) error {
	c, err := client(true)
	if err != nil {
		return err
	}
	if slug == "" {
		slug, err = pickDirectoryClickSlug(ctx, c)
		if err != nil {
			return err
		}
	}
	already, err := c.RequestClickJoin(ctx, slug)
	if err != nil {
		return formatErr(err)
	}
	if already {
		ui.Println("Your join request is already pending.")
		return nil
	}
	ui.Println("Join request sent.")
	return nil
}

func runClickCancelRequest(ctx context.Context, slug string) error {
	c, err := client(true)
	if err != nil {
		return err
	}
	if slug == "" {
		slug, err = pickDirectoryClickSlug(ctx, c)
		if err != nil {
			return err
		}
	}
	ok, err := ui.Confirm("Cancel request")
	if err != nil {
		return err
	}
	if !ok {
		ui.Println("Cancelled.")
		return nil
	}
	if err := c.CancelClickJoin(ctx, slug); err != nil {
		return formatErr(err)
	}
	ui.Println("Join request cancelled.")
	return nil
}

func runClickRequests(ctx context.Context, slug string) error {
	c, err := client(true)
	if err != nil {
		return err
	}
	if slug == "" {
		slug, err = pickClickSlug(ctx, c)
		if err != nil {
			return err
		}
	}
	_, reqs, canModerate, err := c.ClickMembers(ctx, slug)
	if err != nil {
		return formatErr(err)
	}
	if !canModerate {
		return fmt.Errorf("only Click moderators can list join requests")
	}
	if len(reqs) == 0 {
		ui.Println("No pending join requests.")
		return nil
	}
	ui.Printf("Pending join requests for /%s:\n", slug)
	for _, r := range reqs {
		ui.Printf("  %d  @%s  %s\n", r.ID, r.Username, r.CreatedAt)
	}
	ui.Println("Use: cs3 click allow <id> · cs3 click reject <id> · cs3 click allow-all")
	return nil
}

func runClickAllow(ctx context.Context, id int) error {
	c, err := client(true)
	if err != nil {
		return err
	}
	ok, err := ui.Confirm("Allow this join request")
	if err != nil {
		return err
	}
	if !ok {
		ui.Println("Cancelled.")
		return nil
	}
	if err := c.AllowJoinRequest(ctx, id); err != nil {
		return formatErr(err)
	}
	ui.Println("Join request allowed.")
	return nil
}

func runClickReject(ctx context.Context, id int, reason string) error {
	c, err := client(true)
	if err != nil {
		return err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason, err = ui.ReadLine("Reject reason: ")
		if err != nil {
			return err
		}
		reason = strings.TrimSpace(reason)
	}
	if reason == "" {
		return fmt.Errorf("a reason is required")
	}
	ok, err := ui.Confirm("Reject this join request")
	if err != nil {
		return err
	}
	if !ok {
		ui.Println("Cancelled.")
		return nil
	}
	if err := c.RejectJoinRequest(ctx, id, reason); err != nil {
		return formatErr(err)
	}
	ui.Println("Join request rejected.")
	return nil
}

func runClickAllowAll(ctx context.Context, slug string) error {
	c, err := client(true)
	if err != nil {
		return err
	}
	if slug == "" {
		slug, err = pickClickSlug(ctx, c)
		if err != nil {
			return err
		}
	}
	ok, err := ui.Confirm("Approve all pending join requests")
	if err != nil {
		return err
	}
	if !ok {
		ui.Println("Cancelled.")
		return nil
	}
	allowed, skipped, err := c.AllowAllJoinRequests(ctx, slug)
	if err != nil {
		return formatErr(err)
	}
	if skipped > 0 {
		ui.Printf("Approved %d. Skipped %d.\n", allowed, skipped)
		return nil
	}
	if allowed == 1 {
		ui.Println("1 join request approved.")
		return nil
	}
	ui.Printf("%d join requests approved.\n", allowed)
	return nil
}
