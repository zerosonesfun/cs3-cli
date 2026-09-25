package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/zerosonesfun/cs3-cli/internal/api"
	"github.com/zerosonesfun/cs3-cli/internal/auth"
	"github.com/zerosonesfun/cs3-cli/internal/config"
	"github.com/zerosonesfun/cs3-cli/internal/ui"
)

func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "cs3",
		Short:         "Ctrl+Shift+3 command-line client",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMenu(cmd.Context())
		},
	}
	root.AddCommand(
		cmdLogin(),
		cmdLogout(),
		cmdWhoami(),
		cmdFeed(),
		cmdPost(),
		cmdMine(),
		cmdWall(),
		cmdQuote(),
		cmdPings(),
		cmdSearch(),
		cmdClick(),
		cmdSettings(),
		cmdInstallPath(),
	)
	return root
}

func client(needAuth bool) (*api.Client, error) {
	base, err := config.BaseURL()
	if err != nil {
		return nil, err
	}
	tok := ""
	if needAuth {
		tok, err = auth.RequireToken()
		if err != nil {
			return nil, err
		}
	}
	return api.New(base, tok), nil
}

func cmdLogin() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Log in with username (or email) and password",
		RunE: func(cmd *cobra.Command, args []string) error {
			return doLogin(cmd.Context())
		},
	}
}

func doLogin(ctx context.Context) error {
	login, err := ui.ReadLine("Username or email: ")
	if err != nil {
		return err
	}
	pass, err := ui.ReadPassword("Password: ")
	if err != nil {
		return err
	}
	defer clearString(&pass)
	if login == "" || pass == "" {
		clearString(&pass)
		return fmt.Errorf("login and password are required")
	}
	c, err := client(false)
	if err != nil {
		clearString(&pass)
		return err
	}
	token, user, err := c.Login(ctx, login, pass)
	clearString(&pass)
	if err != nil {
		return err
	}
	if err := auth.SaveToken(token); err != nil {
		return fmt.Errorf("could not store token securely: %w", err)
	}
	cfg, _ := config.Load()
	cfg.Username = user.Username
	_ = config.Save(cfg)
	ui.Printf("Logged in as %s.\n", user.Username)
	return nil
}

func clearString(s *string) {
	if s == nil {
		return
	}
	b := []byte(*s)
	for i := range b {
		b[i] = 0
	}
	*s = ""
}

func clearLocalSession() {
	_ = auth.ClearToken()
	cfg, err := config.Load()
	if err != nil {
		return
	}
	cfg.Username = ""
	_ = config.Save(cfg)
}

func formatErr(err error) error {
	var ae *api.APIError
	if errors.As(err, &ae) && ae.Unauthorized() {
		clearLocalSession()
		return fmt.Errorf("%w — run: cs3 login", ae)
	}
	return err
}

func isUnauthorized(err error) bool {
	var ae *api.APIError
	return errors.As(err, &ae) && ae.Unauthorized()
}

func cmdLogout() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Revoke this CLI token and clear local credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if c, err := client(true); err == nil {
				_ = c.Logout(ctx)
			}
			clearLocalSession()
			ui.Println("Logged out.")
			return nil
		},
	}
}

func cmdWhoami() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the logged-in account",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client(true)
			if err != nil {
				return err
			}
			u, err := c.Me(cmd.Context())
			if err != nil {
				return formatErr(err)
			}
			ui.Printf("%s\n", u.Username)
			return nil
		},
	}
}

func cmdFeed() *cobra.Command {
	return &cobra.Command{
		Use:   "feed",
		Short: "Show latest 20 global feed posts; pick a number to read and comment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return browseLatestFeed(cmd.Context())
		},
	}
}

func browseLatestFeed(ctx context.Context) error {
	c, err := client(true)
	if err != nil {
		return err
	}
	posts, err := c.Feed(ctx, 1)
	if err != nil {
		return formatErr(err)
	}
	if len(posts) == 0 {
		ui.Println("No posts.")
		return nil
	}
	printPosts(posts)
	for {
		choice, err := ui.ReadLine("Post # / quote @user (b = back): ")
		if err != nil {
			return err
		}
		choice = strings.TrimSpace(choice)
		if choice == "" || isBackCmd(choice) {
			return nil
		}
		if u, ok := parseQuoteChoice(choice); ok {
			if err := showQuote(ctx, u); err != nil {
				if isUnauthorized(err) {
					return err
				}
				ui.Printf("Error: %v\n", err)
				continue
			}
			posts, err = c.Feed(ctx, 1)
			if err != nil {
				return formatErr(err)
			}
			if len(posts) == 0 {
				ui.Println("No posts.")
				return nil
			}
			printPosts(posts)
			continue
		}
		n, err := strconv.Atoi(choice)
		if err != nil || n < 1 || n > len(posts) {
			ui.Printf("Pick a number between 1 and %d, quote @user, or b to go back.\n", len(posts))
			continue
		}
		back, err := showPostThread(ctx, posts[n-1].ID)
		if err != nil {
			if isUnauthorized(err) {
				return err
			}
			ui.Printf("Error: %v\n", err)
			continue
		}
		if back {
			return nil
		}
	}
}

func isBackCmd(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "b", "m", "0":
		return true
	default:
		return false
	}
}

func isHelpCmd(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "2", "h", "help", "?":
		return true
	default:
		return false
	}
}

func printMenuHelp() {
	ui.Println("Type a menu number. 0 or q quits.")
	ui.Println()
	ui.Println("Feed: type a post number to read and comment.")
	ui.Println("  b goes back (also m, or Enter at the list).")
	ui.Println("  quote @user opens their wall and clears \"N.")
	ui.Println("  Comment: several lines, then a line with only .")
	ui.Println("  b then . skips the comment. A lone . skips posting, then b.")
	ui.Println()
	ui.Println("Pings: type a ping number to open it (moves to Past).")
	ui.Println("  cs3 pings past — opened archive; c clears all past.")
	ui.Println("  Related comment pings for the same post can be dismissed together.")
	ui.Println()
	ui.Println("Thoughts: same . to finish, then confirm. Art and polls are read-only.")
	ui.Println("Up to 3 links per thought or comment.")
	ui.Println()
	ui.Println("Special:")
	ui.Println("  b     back")
	ui.Println("  .     end a thought or comment")
	ui.Println("  h / ? this help")
	ui.Println("  0 / q quit from the main menu")
	ui.Println()
	ui.Println("Also: cs3 quote @username")
	ui.Println("Log out ends this CLI session. Quit does not.")
}

func cmdPost() *cobra.Command {
	// Manual dispatcher so `cs3 post <id>` works alongside create/edit/delete.
	// Cobra treats unknown first args as missing subcommands when AddCommand is used.
	return &cobra.Command{
		Use:   "post [create|edit|delete|<id>]",
		Short: "Show, create, edit, or delete posts",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			switch args[0] {
			case "create":
				if len(args) != 1 {
					return fmt.Errorf("usage: cs3 post create")
				}
				return composePost(cmd.Context())
			case "edit":
				if len(args) != 2 {
					return fmt.Errorf("usage: cs3 post edit <id>")
				}
				return editPost(cmd.Context(), args[1])
			case "delete":
				if len(args) != 2 {
					return fmt.Errorf("usage: cs3 post delete <id>")
				}
				return deletePost(cmd.Context(), args[1], true)
			default:
				if len(args) != 1 {
					return fmt.Errorf("usage: cs3 post <id>")
				}
				_, err := showPostThread(cmd.Context(), args[0])
				return err
			}
		},
	}
}

func showPostThread(ctx context.Context, id string) (backToMenu bool, err error) {
	c, err := client(true)
	if err != nil {
		return false, err
	}
	post, comments, err := c.Post(ctx, id)
	if err != nil {
		return false, formatErr(err)
	}
	meta := formatUsername(post.Username, post.WallQuoteCount)
	if post.IsBubbled {
		meta = "bubbled • " + meta
	}
	ui.Printf("\n%s · %s · %s", meta, post.CreatedAt, post.ID)
	if post.IsBubbled {
		ui.Printf("\n%s", bubbleNotice)
	}
	if post.IsArt {
		ui.Printf(" [art]\n%s\n\n", ui.FormatArt(post.Body, post.ArtColors, false))
	} else if post.IsPoll {
		ui.Printf(" [poll]\n%s\n\n", ui.Emojicon(post.Body))
	} else {
		ui.Printf("\n%s\n\n", ui.Emojicon(post.Body))
	}
	if len(comments) == 0 {
		ui.Println("(no comments)")
	} else {
		ui.Printf("--- comments (%d) ---\n", len(comments))
		for i, cm := range comments {
			if i > 0 {
				ui.Println()
				ui.Println("---")
			}
			ui.Printf("%d. %s · %s\n   %s\n", i+1, formatUsername(cm.Username, cm.WallQuoteCount), cm.CreatedAt, indentBody(ui.Emojicon(cm.Body)))
		}
	}
	ui.Println()
	if post.Status != "" && post.Status != "visible" {
		return waitForBack()
	}
	return promptComment(ctx, c, post.ID)
}

func promptComment(ctx context.Context, c *api.Client, postID string) (backToMenu bool, err error) {
	for {
		ui.Println("Comment (end with . on its own line; b then . to skip):")
		body, err := ui.ReadMultiline(".")
		if err != nil {
			return false, err
		}
		if isBackCmd(body) {
			return true, nil
		}
		if strings.TrimSpace(body) == "" {
			break
		}
		if err := api.ValidateCommentBody(body); err != nil {
			ui.Printf("%v\n", err)
			continue
		}
		key := api.NewIdempotencyKey()
		if _, err := c.CreateComment(ctx, postID, body, key); err != nil {
			err = formatErr(err)
			if isUnauthorized(err) {
				return false, err
			}
			ui.Printf("Error: %v\n", err)
			var ae *api.APIError
			if errors.As(err, &ae) && (ae.Status == 403 || ae.Status == 404 || ae.Status >= 500) {
				break
			}
			continue
		}
		ui.Println("Posted.")
		break
	}
	return waitForBack()
}

func waitForBack() (backToMenu bool, err error) {
	for {
		choice, err := ui.ReadLine("b = back: ")
		if err != nil {
			return true, err
		}
		if isBackCmd(choice) || choice == "" {
			return true, nil
		}
		ui.Println("Type b to go back.")
	}
}

func cmdWall() *cobra.Command {
	return &cobra.Command{
		Use:   "wall",
		Short: "Show posts on your profile wall",
		RunE: func(cmd *cobra.Command, args []string) error {
			return showWall(cmd.Context())
		},
	}
}

func cmdQuote() *cobra.Command {
	return &cobra.Command{
		Use:   "quote @username",
		Short: "Open an author’s profile wall (clears unread wall quote)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return showQuote(cmd.Context(), args[0])
		},
	}
}

func showQuote(ctx context.Context, username string) error {
	c, err := client(true)
	if err != nil {
		return err
	}
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	count, err := c.WallQuote(ctx, username)
	if err != nil {
		return formatErr(err)
	}
	posts, err := c.Wall(ctx, username, 1)
	if err != nil {
		return formatErr(err)
	}
	label := strconv.Itoa(count)
	if count > 10 {
		label = "10+"
	}
	ui.Printf("Wall for @%s (%s new)\n", username, label)
	if len(posts) == 0 {
		ui.Println("No wall posts.")
		return nil
	}
	printPosts(posts)
	return nil
}

func parseQuoteChoice(s string) (username string, ok bool) {
	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)
	if !strings.HasPrefix(lower, "quote ") {
		return "", false
	}
	u := strings.TrimSpace(s[len("quote "):])
	u = strings.TrimPrefix(u, "@")
	if u == "" || strings.ContainsAny(u, "/\\?") {
		return "", false
	}
	return u, true
}

func showWall(ctx context.Context) error {
	c, err := client(true)
	if err != nil {
		return err
	}
	u, err := c.Me(ctx)
	if err != nil {
		return formatErr(err)
	}
	posts, err := c.Wall(ctx, u.Username, 1)
	if err != nil {
		return formatErr(err)
	}
	ui.Printf("Wall for @%s\n", u.Username)
	if len(posts) == 0 {
		ui.Println("No wall posts.")
		return nil
	}
	printPosts(posts)
	return nil
}

func cmdPings() *cobra.Command {
	// Manual dispatcher so `cs3 pings past` cannot become an unknown subcommand
	// (same pattern as cmdPost).
	return &cobra.Command{
		Use:   "pings [past]",
		Short: "List pings (or past archive); type a number to open one",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return showPings(cmd.Context())
			}
			if strings.EqualFold(args[0], "past") {
				if len(args) != 1 {
					return fmt.Errorf("usage: cs3 pings past")
				}
				return showPastPings(cmd.Context())
			}
			return fmt.Errorf("unknown pings argument %q (try: cs3 pings or cs3 pings past)", args[0])
		},
	}
}

func isCommentPingKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "post_comment", "comment_reply":
		return true
	default:
		return false
	}
}

func relatedCommentCount(pings []api.Ping, ping api.Ping) int {
	if !isCommentPingKind(ping.Kind) || ping.PostID == "" {
		return 0
	}
	n := 0
	for _, p := range pings {
		if isCommentPingKind(p.Kind) && p.PostID == ping.PostID {
			n++
		}
	}
	if ping.RelatedCommentCount > n {
		return ping.RelatedCommentCount
	}
	return n
}

func showPings(ctx context.Context) error {
	c, err := client(true)
	if err != nil {
		return err
	}
	for {
		pings, count, err := c.Pings(ctx)
		if err != nil {
			return formatErr(err)
		}
		cfg, _ := config.Load()
		var maxID int64
		var newCount int
		for _, p := range pings {
			if p.ID > maxID {
				maxID = p.ID
			}
			if p.ID > cfg.LastSeenPingID {
				newCount++
			}
		}
		if count == 0 || len(pings) == 0 {
			ui.Println("No pings.")
			ui.Println("Tip: type p for past, or: cs3 pings past")
			if maxID > cfg.LastSeenPingID {
				cfg.LastSeenPingID = maxID
				_ = config.Save(cfg)
			}
			for {
				choice, err := ui.ReadLine("p = past, b = back: ")
				if err != nil {
					return err
				}
				choice = strings.TrimSpace(choice)
				if choice == "" || isBackCmd(choice) {
					return nil
				}
				if strings.EqualFold(choice, "p") || strings.EqualFold(choice, "past") {
					if err := showPastPings(ctx); err != nil {
						return err
					}
					break
				}
				ui.Println("Type p for past pings, or b to go back.")
			}
			continue
		}
		ui.Printf("%d ping(s) in inbox", count)
		if newCount > 0 {
			ui.Printf(" · %d new since last check", newCount)
		} else {
			ui.Printf(" · no new since last check")
		}
		ui.Println()
		for i, p := range pings {
			mark := " "
			if p.ID > cfg.LastSeenPingID {
				mark = "*"
			}
			printPingLine(i+1, mark, p)
		}
		if maxID > cfg.LastSeenPingID {
			cfg.LastSeenPingID = maxID
			_ = config.Save(cfg)
		}

		for {
			choice, err := ui.ReadLine("Ping # (p = past, b = back): ")
			if err != nil {
				return err
			}
			choice = strings.TrimSpace(choice)
			if choice == "" || isBackCmd(choice) {
				return nil
			}
			if strings.EqualFold(choice, "p") || strings.EqualFold(choice, "past") {
				if err := showPastPings(ctx); err != nil {
					return err
				}
				break
			}
			n, err := strconv.Atoi(choice)
			if err != nil || n < 1 || n > len(pings) {
				ui.Printf("Pick a number between 1 and %d, p for past, or b to go back.\n", len(pings))
				continue
			}
			back, err := openPing(ctx, c, pings, pings[n-1])
			if err != nil {
				if isUnauthorized(err) {
					return err
				}
				ui.Printf("Error: %v\n", err)
				break
			}
			if back {
				return nil
			}
			break
		}
	}
}

func printPingLine(n int, mark string, p api.Ping) {
	if p.Kind == "click_posts" {
		ui.Printf("%d.%s %s\n   %s\n", n, mark, p.Summary, p.CreatedAt)
		return
	}
	ui.Printf("%d.%s %s\n   @%s · %s\n", n, mark, p.Summary, p.ActorUsername, p.CreatedAt)
}

func printPastPingLine(n int, p api.Ping, when string) {
	if p.Kind == "click_posts" {
		ui.Printf("%d. %s\n   %s\n", n, p.Summary, when)
		return
	}
	ui.Printf("%d. %s\n   @%s · %s\n", n, p.Summary, p.ActorUsername, when)
}

func showClickPosts(ctx context.Context, c *api.Client, slug string) error {
	click, posts, gate, hasMore, err := c.Click(ctx, slug)
	if err != nil {
		return err
	}
	name := click.Name
	if name == "" {
		name = slug
	}
	ui.Println(name)
	if gate {
		ui.Println("You are not a member of this Click.")
		return nil
	}
	if len(posts) == 0 {
		ui.Println("No posts.")
		return nil
	}
	printPosts(posts)
	if hasMore {
		ui.Println()
		ui.Println("More posts in this Click.")
	}
	return nil
}

func openPing(ctx context.Context, c *api.Client, pings []api.Ping, ping api.Ping) (backToMenu bool, err error) {
	dismissRelated := false
	if relatedCommentCount(pings, ping) > 1 {
		ok, err := ui.Confirm("There are multiple comment pings related to this post. Go ahead and dismiss all related pings since you're going there now?")
		if err != nil {
			return false, err
		}
		dismissRelated = ok
	}
	target, err := c.PingGo(ctx, ping.ID, dismissRelated)
	if err != nil {
		return false, formatErr(err)
	}
	switch target.Type {
	case "post":
		if target.ID != "" {
			return showPostThread(ctx, target.ID)
		}
	case "click":
		if target.Slug != "" {
			if err := showClickPosts(ctx, c, target.Slug); err != nil {
				return false, err
			}
		}
	case "url":
		if target.URL != "" {
			ui.Printf("Polling Place is on the website: %s\n", target.URL)
		} else {
			ui.Println("Ping cleared.")
		}
	default:
		ui.Println("Ping cleared.")
	}
	return false, nil
}

func showPastPings(ctx context.Context) error {
	c, err := client(true)
	if err != nil {
		return err
	}
	page := 1
	var all []api.Ping
	for {
		pings, hasMore, err := c.PingsPast(ctx, page)
		if err != nil {
			return formatErr(err)
		}
		if page == 1 && len(pings) == 0 {
			ui.Println("No past pings.")
			return nil
		}
		all = append(all, pings...)
		for i, p := range pings {
			n := len(all) - len(pings) + i + 1
			when := p.OpenedAt
			if when == "" {
				when = p.CreatedAt
			}
			printPastPingLine(n, p, when)
		}
		if hasMore {
			more, err := ui.Confirm("Load more")
			if err != nil {
				return err
			}
			if more {
				page++
				continue
			}
		}
		break
	}

	for {
		choice, err := ui.ReadLine("Ping # (c = clear all, b = back): ")
		if err != nil {
			return err
		}
		choice = strings.TrimSpace(choice)
		if choice == "" || isBackCmd(choice) {
			return nil
		}
		if strings.EqualFold(choice, "c") || strings.EqualFold(choice, "clear") {
			ok, err := ui.Confirm("Delete all past pings? This cannot be undone")
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			if err := c.PingsPastClear(ctx); err != nil {
				return formatErr(err)
			}
			ui.Println("Past pings cleared.")
			return nil
		}
		n, err := strconv.Atoi(choice)
		if err != nil || n < 1 || n > len(all) {
			ui.Printf("Pick a number between 1 and %d, c to clear all, or b to go back.\n", len(all))
			continue
		}
		_, err = openPing(ctx, c, []api.Ping{all[n-1]}, all[n-1])
		if err != nil {
			if isUnauthorized(err) {
				return err
			}
			ui.Printf("Error: %v\n", err)
			continue
		}
	}
}

func cmdSettings() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "settings",
		Short: "Show or change account preferences",
		RunE: func(cmd *cobra.Command, args []string) error {
			return settingsInteractive(cmd.Context())
		},
	}
	cmd.AddCommand(
		settingsGet(),
		settingsSet(),
	)
	return cmd
}

func settingsGet() *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "Print current settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client(true)
			if err != nil {
				return err
			}
			u, err := c.Me(cmd.Context())
			if err != nil {
				return formatErr(err)
			}
			printSettingsLoaded(cmd.Context(), c, u)
			return nil
		},
	}
}

func settingsSet() *cobra.Command {
	var (
		theme, font, timezone, petName, username, password                                                      string
		sound, profilePosts, blockInvites, pings, postCommentPings, commentReplyPings, digest, hideBubbles, fed string
	)
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Update one or more settings (use on/off for booleans)",
		RunE: func(cmd *cobra.Command, args []string) error {
			patch := map[string]any{}
			if theme != "" {
				patch["ui_theme"] = theme
			}
			if font != "" {
				patch["ui_font"] = font
			}
			if timezone != "" {
				patch["timezone"] = timezone
			}
			boolFields := map[string]*string{
				"sound_enabled":             &sound,
				"allow_profile_posts":       &profilePosts,
				"block_click_invites":       &blockInvites,
				"allow_pings":               &pings,
				"allow_post_comment_pings":  &postCommentPings,
				"allow_comment_reply_pings": &commentReplyPings,
				"email_ping_digest":         &digest,
				"hide_bubbled_posts":        &hideBubbles,
				"federate_global":           &fed,
			}
			for k, ptr := range boolFields {
				if *ptr == "" {
					continue
				}
				v, err := parseOnOff(*ptr)
				if err != nil {
					return fmt.Errorf("%s: %w", k, err)
				}
				patch[k] = v
			}
			hasPetName := cmd.Flags().Changed("pet-name")
			if hasPetName {
				petName = strings.TrimSpace(petName)
				if petName == "" {
					return fmt.Errorf("pet name is required")
				}
			}
			hasUsername := cmd.Flags().Changed("username")
			if hasUsername {
				username = strings.TrimSpace(username)
				if username == "" {
					return fmt.Errorf("username is required")
				}
			}
			if len(patch) == 0 && !hasPetName && !hasUsername {
				return fmt.Errorf("pass at least one flag (see cs3 settings set -h)")
			}
			c, err := client(true)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			var (
				u          api.User
				pet        *api.Pet
				petErr     error
				prefErr    error
				userErr    error
				savedPet   bool
				savedPrefs bool
				savedUser  bool
			)
			if hasPetName {
				named, err := c.NamePet(ctx, petName)
				if err != nil {
					petErr = err
				} else {
					savedPet = true
					pet = &named
				}
			}
			if hasUsername {
				pass := password
				if pass == "" {
					var perr error
					pass, perr = ui.ReadPassword("Current password: ")
					if perr != nil {
						return perr
					}
				}
				if strings.TrimSpace(pass) == "" {
					return fmt.Errorf("password is required")
				}
				updated, err := c.UpdateUsername(ctx, username, pass)
				if err != nil {
					userErr = err
				} else {
					savedUser = true
					u = updated
				}
			}
			if len(patch) > 0 {
				var warnings []string
				u, warnings, prefErr = c.UpdateSettings(ctx, patch)
				if prefErr == nil {
					savedPrefs = true
					for _, w := range warnings {
						ui.Printf("Warning: %s\n", w)
					}
				}
			}
			if u.Username == "" {
				if me, err := c.Me(ctx); err == nil {
					u = me
				}
			}
			var loadErr error
			if pet == nil {
				pet, loadErr = fetchPetPeek(ctx, c)
			}
			allOk := petErr == nil && prefErr == nil && userErr == nil
			if allOk {
				ui.Println("Saved.")
			} else {
				if savedUser {
					ui.Println("Username saved.")
				}
				if savedPet {
					ui.Println("Pet name saved.")
				}
				if savedPrefs {
					ui.Println("Preferences saved.")
				}
			}
			if u.Username != "" {
				printSettings(u, pet, loadErr)
			}
			errs := []string{}
			if userErr != nil {
				errs = append(errs, fmt.Sprintf("username: %v", formatErr(userErr)))
			}
			if petErr != nil {
				errs = append(errs, fmt.Sprintf("pet name: %v", formatErr(petErr)))
			}
			if prefErr != nil {
				errs = append(errs, fmt.Sprintf("preferences: %v", formatErr(prefErr)))
			}
			if len(errs) > 0 {
				return fmt.Errorf("%s", strings.Join(errs, "; "))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&theme, "theme", "", "system|light|dark")
	cmd.Flags().StringVar(&font, "font", "", "courier|ibm-plex-mono|source-code-pro|jetbrains-mono|atkinson-hyperlegible|literata")
	cmd.Flags().StringVar(&timezone, "timezone", "", "IANA timezone for features like bubbled posts (e.g. America/New_York)")
	cmd.Flags().StringVar(&sound, "sound", "", "on|off - website UI sounds")
	cmd.Flags().StringVar(&profilePosts, "profile-posts", "", "on|off - allow visitors on profile")
	cmd.Flags().StringVar(&blockInvites, "block-invites", "", "on|off - block directed Click invitations")
	cmd.Flags().StringVar(&pings, "pings", "", "on|off - @mention pings")
	cmd.Flags().StringVar(&postCommentPings, "post-comment-pings", "", "on|off - pings when someone comments on your posts")
	cmd.Flags().StringVar(&commentReplyPings, "comment-reply-pings", "", "on|off - pings when someone comments after you")
	cmd.Flags().StringVar(&digest, "email-digest", "", "on|off - daily unread ping email")
	cmd.Flags().StringVar(&hideBubbles, "hide-bubbles", "", "on|off - hide bubbled posts from main feed")
	cmd.Flags().StringVar(&fed, "federate", "", "on|off - send main feed to Fediverse")
	cmd.Flags().StringVar(&petName, "pet-name", "", "companion name (max 24)")
	cmd.Flags().StringVar(&username, "username", "", "new username (max 3 lifetime changes; prompts for password)")
	cmd.Flags().StringVar(&password, "password", "", "current password (optional with --username; otherwise prompted)")
	return cmd
}

func settingsInteractive(ctx context.Context) error {
	c, err := client(true)
	if err != nil {
		return err
	}
	u, err := c.Me(ctx)
	if err != nil {
		return formatErr(err)
	}
	printSettingsLoaded(ctx, c, u)
	ui.Println()
	ui.Println("1) Theme")
	ui.Println("2) Font")
	ui.Println("Bubbled Posts")
	ui.Println("  (old posts that get bubbled to the top of the main feed once a day)")
	ui.Println("3) Toggle hide bubbled posts")
	ui.Println("4) Timezone")
	ui.Println("5) Toggle UI sounds")
	ui.Println("6) Toggle visitors on profile")
	ui.Println("7) Toggle block Click invitations")
	ui.Println("8) Toggle @mention pings")
	ui.Println("9) Toggle comments-on-your-posts pings")
	ui.Println("10) Toggle comments-after-you pings")
	ui.Println("11) Toggle email ping digest")
	ui.Println("12) Toggle Fediverse sharing")
	ui.Println("13) Pet name")
	ui.Println("14) Change username (max 3 times)")
	ui.Println("0) Back")
	choice, err := ui.ReadLine("Choice: ")
	if err != nil || choice == "" || choice == "0" {
		return nil
	}
	if choice == "13" {
		return setPetNameInteractive(ctx, c, u)
	}
	if choice == "14" {
		return setUsernameInteractive(ctx, c, u)
	}
	patch := map[string]any{}
	switch choice {
	case "1":
		v, err := ui.ReadLine("Theme (system|light|dark): ")
		if err != nil {
			return err
		}
		if v == "" {
			return fmt.Errorf("theme is required")
		}
		patch["ui_theme"] = v
	case "2":
		ui.Println("Fonts: courier, ibm-plex-mono, source-code-pro, jetbrains-mono, atkinson-hyperlegible, literata")
		v, err := ui.ReadLine("Font: ")
		if err != nil {
			return err
		}
		if v == "" {
			return fmt.Errorf("font is required")
		}
		patch["ui_font"] = v
	case "3":
		patch["hide_bubbled_posts"] = !u.HideBubbledPosts
	case "4":
		v, err := ui.ReadLine(fmt.Sprintf("Timezone IANA (blank = %s): ", api.LocalTimezone()))
		if err != nil {
			return err
		}
		if v == "" {
			v = api.LocalTimezone()
		}
		patch["timezone"] = v
	case "5":
		patch["sound_enabled"] = !u.SoundEnabled
	case "6":
		patch["allow_profile_posts"] = !u.AllowProfilePosts
	case "7":
		patch["block_click_invites"] = !u.BlockClickInvites
	case "8":
		patch["allow_pings"] = !u.AllowPings
	case "9":
		patch["allow_post_comment_pings"] = !u.AllowPostCommentPings
	case "10":
		patch["allow_comment_reply_pings"] = !u.AllowCommentReplyPings
	case "11":
		patch["email_ping_digest"] = !u.EmailPingDigest
	case "12":
		patch["federate_global"] = !u.FederateGlobal
	default:
		return fmt.Errorf("unknown choice")
	}
	u2, warnings, err := c.UpdateSettings(ctx, patch)
	if err != nil {
		return formatErr(err)
	}
	for _, w := range warnings {
		ui.Printf("Warning: %s\n", w)
	}
	ui.Println("Saved.")
	printSettingsLoaded(ctx, c, u2)
	return nil
}

func setPetNameInteractive(ctx context.Context, c *api.Client, u api.User) error {
	current, err := fetchPetPeek(ctx, c)
	if err != nil {
		ui.Printf("Current pet name: (unavailable)\n")
		ui.Printf("Warning: could not load pet name: %v\n", formatErr(err))
	} else {
		ui.Printf("Current pet name: %s\n", formatPetName(current, nil))
	}
	v, err := ui.ReadLine("Pet name: ")
	if err != nil {
		return err
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return fmt.Errorf("pet name is required")
	}
	named, err := c.NamePet(ctx, v)
	if err != nil {
		return formatErr(err)
	}
	ui.Println("Saved.")
	printSettings(u, &named, nil)
	return nil
}

func setUsernameInteractive(ctx context.Context, c *api.Client, u api.User) error {
	ui.Printf("Current username: @%s\n", u.Username)
	ui.Println("You can change your username up to 3 times.")
	ui.Printf("%d of 3 changes left.\n", u.UsernameChangesRemaining)
	if u.UsernameChangesRemaining < 1 {
		ui.Println("You have used all 3 username changes.")
		return nil
	}
	name, err := ui.ReadLine("New username: ")
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("username is required")
	}
	pass, err := ui.ReadPassword("Current password: ")
	if err != nil {
		return err
	}
	if strings.TrimSpace(pass) == "" {
		return fmt.Errorf("password is required")
	}
	updated, err := c.UpdateUsername(ctx, name, pass)
	if err != nil {
		return formatErr(err)
	}
	ui.Println("Saved.")
	printSettingsLoaded(ctx, c, updated)
	return nil
}

func cmdInstallPath() *cobra.Command {
	return &cobra.Command{
		Use:   "install-path",
		Short: "Copy this binary into a user directory on PATH",
		RunE: func(cmd *cobra.Command, args []string) error {
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			exe, err = filepath.EvalSymlinks(exe)
			if err != nil {
				return err
			}
			var destDir string
			switch runtime.GOOS {
			case "windows":
				base := os.Getenv("LOCALAPPDATA")
				if base == "" {
					home, err := os.UserHomeDir()
					if err != nil {
						return err
					}
					base = filepath.Join(home, "AppData", "Local")
				}
				destDir = filepath.Join(base, "cs3")
			default:
				home, err := os.UserHomeDir()
				if err != nil {
					return err
				}
				destDir = filepath.Join(home, "bin")
			}
			if err := os.MkdirAll(destDir, 0o755); err != nil {
				return err
			}
			name := "cs3"
			if runtime.GOOS == "windows" {
				name = "cs3.exe"
			}
			dest := filepath.Join(destDir, name)
			if err := copyFileAtomic(exe, dest, 0o755); err != nil {
				return err
			}
			ui.Printf("Installed to %s\n", dest)
			ui.Println("Ensure that directory is on your PATH, then reopen the terminal.")
			if runtime.GOOS != "windows" {
				ui.Printf("Example: echo 'export PATH=\"$HOME/bin:$PATH\"' >> ~/.zshrc\n")
			}
			return nil
		},
	}
}

func copyFileAtomic(src, dest string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	dir := filepath.Dir(dest)
	tmp, err := os.CreateTemp(dir, ".tmp-cs3-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := io.Copy(tmp, in); err != nil {
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return err
	}
	ok = true
	return nil
}

func printPet(ctx context.Context, c *api.Client) {
	p, err := c.Pet(ctx)
	if err != nil || p.Face == "" {
		return
	}
	line := p.Face
	if name := formatPetName(&p, nil); name != "(unnamed)" {
		line += "  " + name
	}
	if p.Mood != "" {
		line += "  (" + p.Mood + ")"
	}
	ui.Println(line)
	if p.Speech != nil {
		speech := strings.TrimSpace(*p.Speech)
		if speech != "" {
			// Hungry API copy mentions press-and-hold for web/iOS; CLI has no feed gesture.
			if p.Mood == "hungry" {
				if i := strings.IndexByte(speech, '\n'); i >= 0 {
					speech = strings.TrimSpace(speech[:i])
				}
			}
			ui.Println(speech)
		}
	}
}

func runMenu(ctx context.Context) error {
	ui.PrintBanner()
	base, _ := config.BaseURL()
	ui.Printf("Server: %s\n", base)

	_, err := auth.LoadToken()
	loggedIn := err == nil
	if loggedIn {
		if c, err := client(true); err == nil {
			if u, err := c.Me(ctx); err == nil {
				ui.Printf("Signed in as @%s\n", u.Username)
				printPet(ctx, c)
			} else {
				ui.Println("Session expired or invalid — please log in again.")
				clearLocalSession()
				loggedIn = false
			}
		}
	} else {
		ui.Println("Not signed in.")
	}
	ui.Println()

	for {
		if !loggedIn {
			ui.Println("1) Log in")
			ui.Println("2) Help")
			ui.Println("0) Quit")
			choice, err := ui.ReadLine("> ")
			if err != nil || choice == "0" || choice == "q" || choice == "quit" {
				return nil
			}
			if isHelpCmd(choice) {
				printMenuHelp()
				ui.Println()
				continue
			}
			if choice == "1" {
				if err := doLogin(ctx); err != nil {
					ui.Printf("Error: %v\n", err)
					continue
				}
				loggedIn = true
			}
			continue
		}

		ui.Println("1) Latest feed")
		ui.Println("2) Help")
		ui.Println("3) Compose a thought")
		ui.Println("4) My posts")
		ui.Println("5) My profile wall")
		ui.Println("6) Pings")
		ui.Println("7) Search")
		ui.Println("8) Settings")
		ui.Println("9) Log out")
		ui.Println("0) Quit")
		choice, err := ui.ReadLine("> ")
		if err != nil || choice == "0" || choice == "q" || choice == "quit" {
			return nil
		}
		var runErr error
		if isHelpCmd(choice) {
			printMenuHelp()
		} else {
			switch choice {
			case "1":
				runErr = browseLatestFeed(ctx)
			case "3":
				runErr = composePost(ctx)
			case "4":
				runErr = showMine(ctx, true)
			case "5":
				runErr = showWall(ctx)
			case "6":
				runErr = showPings(ctx)
			case "7":
				q, err := ui.ReadLine("Query: ")
				if err != nil {
					runErr = err
				} else {
					q = strings.TrimSpace(q)
					if utf8.RuneCountInString(q) < 2 {
						runErr = fmt.Errorf("query must be at least 2 characters")
					} else {
						runErr = showSearch(ctx, q)
					}
				}
			case "8":
				runErr = settingsInteractive(ctx)
			case "9":
				if c, err := client(true); err == nil {
					_ = c.Logout(ctx)
				}
				clearLocalSession()
				loggedIn = false
				ui.Println("Logged out.")
			default:
				ui.Println("Unknown choice.")
			}
		}
		if runErr != nil {
			ui.Printf("Error: %v\n", runErr)
			if isUnauthorized(runErr) {
				loggedIn = false
			}
		}
		ui.Println()
	}
}

func printPosts(posts []api.Post) {
	for i, p := range posts {
		if i > 0 {
			ui.Println()
			ui.Println("---")
		}
		kind := ""
		snippet := ui.Truncate(ui.Emojicon(p.Body), 120)
		if p.IsArt {
			kind = "art · "
			snippet = ui.FormatArt(p.Body, p.ArtColors, true)
		} else if p.IsPoll {
			kind = "poll · "
		}
		meta := formatUsername(p.Username, p.WallQuoteCount)
		if p.IsBubbled {
			meta = "bubbled • " + meta
		}
		ui.Printf("%2d. %s · %s%s · %d comments · %s\n",
			i+1, meta, kind, p.CreatedAt, p.CommentCount, p.ID)
		if p.IsBubbled {
			ui.Printf("    %s\n", bubbleNotice)
		}
		ui.Printf("    %s\n", snippet)
	}
}

const bubbleNotice = "This post didn't get enough love, so we bubbled it back up for a bit."

func formatUsername(username string, wallQuoteCount int) string {
	if wallQuoteCount > 0 {
		label := strconv.Itoa(wallQuoteCount)
		if wallQuoteCount > 10 {
			label = "10+"
		}
		return "@" + username + " \"" + label
	}
	return "@" + username
}

func printSettingsLoaded(ctx context.Context, c *api.Client, u api.User) {
	pet, err := fetchPetPeek(ctx, c)
	printSettings(u, pet, err)
}

func printSettings(u api.User, pet *api.Pet, petErr error) {
	ui.Printf("Account: @%s\n", u.Username)
	ui.Printf("  username changes:      %d of 3 left (max 3 lifetime)\n", u.UsernameChangesRemaining)
	ui.Printf("  theme:                 %s\n", u.UITheme)
	ui.Printf("  font:                  %s\n", u.UIFont)
	ui.Println("Bubbled Posts:")
	ui.Println("  (old posts that get bubbled to the top of the main feed once a day)")
	ui.Printf("  hide bubbled posts:    %s\n", onOff(u.HideBubbledPosts))
	tz := u.Timezone
	if tz == "" {
		tz = "auto (" + api.LocalTimezone() + ")"
	}
	ui.Printf("  timezone:              %s\n", tz)
	ui.Printf("  sound:                 %s\n", onOff(u.SoundEnabled))
	ui.Printf("  profile posts:         %s\n", onOff(u.AllowProfilePosts))
	ui.Printf("  block invites:         %s\n", onOff(u.BlockClickInvites))
	ui.Printf("  mention pings:         %s\n", onOff(u.AllowPings))
	ui.Printf("  post comment pings:    %s\n", onOff(u.AllowPostCommentPings))
	ui.Printf("  comment reply pings:   %s\n", onOff(u.AllowCommentReplyPings))
	ui.Printf("  email ping digest:     %s\n", onOff(u.EmailPingDigest))
	ui.Printf("  federate global:       %s\n", onOff(u.FederateGlobal))
	ui.Printf("  pet name:              %s\n", formatPetName(pet, petErr))
	if petErr != nil {
		ui.Printf("Warning: could not load pet name: %v\n", formatErr(petErr))
	}
}

func fetchPetPeek(ctx context.Context, c *api.Client) (*api.Pet, error) {
	p, err := c.PetPeek(ctx)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func formatPetName(pet *api.Pet, petErr error) string {
	if petErr != nil {
		return "(unavailable)"
	}
	if pet == nil || !pet.Named || pet.Name == nil {
		return "(unnamed)"
	}
	name := strings.Join(strings.Fields(*pet.Name), " ")
	if name == "" {
		return "(unnamed)"
	}
	return name
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func parseOnOff(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "on", "1", "true", "yes":
		return true, nil
	case "off", "0", "false", "no":
		return false, nil
	default:
		return false, fmt.Errorf("use on or off")
	}
}

func indentBody(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	parts := strings.Split(s, "\n")
	for i := range parts {
		if i == 0 {
			continue
		}
		parts[i] = "   " + parts[i]
	}
	return strings.Join(parts, "\n")
}
