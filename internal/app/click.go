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
		Short: "Create Clicks, search, join requests, or edit a description you own",
	}
	click.AddCommand(
		cmdClickCreate(),
		cmdClickList(),
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

func cmdClickCreate() *cobra.Command {
	return &cobra.Command{
		Use:   "create [name]",
		Short: "Create a Click",
		Long: `Create a Click. Names are unique (letter case does not matter).

If name is omitted, you are prompted. Then type an optional description and end with a line containing only .
An empty description leaves it blank (you can set it later with cs3 click description --set).

Examples:
  cs3 click create
  cs3 click create "Night writers"`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := strings.TrimSpace(strings.Join(args, " "))
			return runClickCreate(cmd.Context(), name)
		},
	}
}

func runClickCreate(ctx context.Context, name string) error {
	c, err := client(true)
	if err != nil {
		return err
	}
	if name == "" {
		name, err = ui.ReadLine("Click name: ")
		if err != nil {
			return err
		}
		name = strings.TrimSpace(name)
	}
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if utf8.RuneCountInString(name) > 80 {
		return fmt.Errorf("name is too long (max 80 characters)")
	}
	ui.Println("Description (end with a line containing only .). An empty description is fine.")
	description, err := ui.ReadMultiline(".")
	if err != nil {
		return err
	}
	click, err := c.CreateClick(ctx, name, description)
	if err != nil {
		return formatErr(err)
	}
	ui.Printf("Created Click %s (/%s).\n", click.Name, click.Slug)
	if click.Description != "" {
		ui.Println(click.Description)
	}
	return nil
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

func printDirectoryClickLine(n int, cl api.Click) {
	status := ""
	if cl.IsMember {
		status = " (member)"
	} else if cl.IsKicked {
		status = " (invite required)"
	} else if cl.JoinPending {
		status = " (requested)"
	}
	ui.Printf("%2d. %s (/%s)%s\n", n, cl.Name, cl.Slug, status)
}

// loadDirectorySection prints title + rows. numberFrom is the first index to print;
// the next unused index is returned as nextNum.
func loadDirectorySection(
	ctx context.Context,
	c *api.Client,
	section, title string,
	numberFrom int,
) (clicks []api.Click, nextNum int, err error) {
	if numberFrom < 1 {
		numberFrom = 1
	}
	page := 1
	n := numberFrom
	var all []api.Click
	for {
		res, err := c.ListClicks(ctx, section, page)
		if err != nil {
			return nil, numberFrom, formatErr(err)
		}
		if page == 1 {
			ui.Println(title)
			if len(res.Clicks) == 0 {
				ui.Println("  None.")
				return all, n, nil
			}
		}
		for _, cl := range res.Clicks {
			all = append(all, cl)
			printDirectoryClickLine(n, cl)
			n++
		}
		if !res.HasMore {
			break
		}
		more, err := ui.Confirm("Load more")
		if err != nil {
			return nil, n, err
		}
		if !more {
			break
		}
		page++
	}
	return all, n, nil
}

func cmdClickList() *cobra.Command {
	return &cobra.Command{
		Use:   "list [new|joined|not-joined]",
		Short: "Browse the Clicks directory (no argument: not joined, then joined)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			which := ""
			if len(args) > 0 {
				which = strings.ToLower(strings.TrimSpace(args[0]))
			}
			return runClickList(cmd.Context(), which)
		},
	}
}

func runClickList(ctx context.Context, which string) error {
	c, err := client(true)
	if err != nil {
		return err
	}
	switch which {
	case "":
	case "new":
		_, _, err := loadDirectorySection(ctx, c, "new", "New", 1)
		return err
	case "joined":
		_, _, err := loadDirectorySection(ctx, c, "joined", "Joined", 1)
		return err
	case "not-joined", "not_joined":
		_, _, err := loadDirectorySection(ctx, c, "not_joined", "Not Joined", 1)
		return err
	default:
		return fmt.Errorf("unknown list %q (use new, joined, or not-joined)", which)
	}
	if _, _, err := loadDirectorySection(ctx, c, "not_joined", "Not Joined", 1); err != nil {
		return err
	}
	ui.Println("")
	if _, _, err := loadDirectorySection(ctx, c, "joined", "Joined", 1); err != nil {
		return err
	}
	return nil
}

func pickDirectoryClickSlug(ctx context.Context, c *api.Client) (string, error) {
	notJoined, next, err := loadDirectorySection(ctx, c, "not_joined", "Not Joined", 1)
	if err != nil {
		return "", err
	}
	ui.Println("")
	joined, _, err := loadDirectorySection(ctx, c, "joined", "Joined", next)
	if err != nil {
		return "", err
	}
	all := append(append([]api.Click{}, notJoined...), joined...)
	if len(all) == 0 {
		return "", fmt.Errorf("no Clicks found")
	}
	pick, err := ui.ReadLine("Click #: ")
	if err != nil {
		return "", err
	}
	n, err := parsePick(pick, len(all))
	if err != nil {
		return "", err
	}
	return all[n-1].Slug, nil
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
