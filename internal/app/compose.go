package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/zerosonesfun/cs3-cli/internal/api"
	"github.com/zerosonesfun/cs3-cli/internal/ui"
	"github.com/spf13/cobra"
)

func cmdMine() *cobra.Command {
	return &cobra.Command{
		Use:   "mine",
		Short: "List your posts across global, wall, and Clicks",
		RunE: func(cmd *cobra.Command, args []string) error {
			return showMine(cmd.Context(), true)
		},
	}
}

func composePost(ctx context.Context) error {
	c, err := client(true)
	if err != nil {
		return err
	}
	me, err := c.Me(ctx)
	if err != nil {
		return formatErr(err)
	}

	ui.Println("Where should this go?")
	ui.Println("1) Main feed (global)")
	ui.Println("2) My profile wall")
	ui.Println("3) A Click I belong to")
	choice, err := ui.ReadLine("Choice: ")
	if err != nil {
		return err
	}
	dest := strings.TrimSpace(choice)
	if dest != "1" && dest != "2" && dest != "3" {
		return fmt.Errorf("pick 1, 2, or 3")
	}

	clickSlug := ""
	clickName := ""
	if dest == "3" {
		clicks, err := c.MyClicks(ctx)
		if err != nil {
			return formatErr(err)
		}
		if len(clicks) == 0 {
			return fmt.Errorf("you are not in any Clicks")
		}
		for i, cl := range clicks {
			ui.Printf("%2d. %s (%s)\n", i+1, cl.Name, cl.Slug)
		}
		pick, err := ui.ReadLine("Click #: ")
		if err != nil {
			return err
		}
		n, err := strconv.Atoi(strings.TrimSpace(pick))
		if err != nil || n < 1 || n > len(clicks) {
			return fmt.Errorf("pick a number between 1 and %d", len(clicks))
		}
		clickSlug = clicks[n-1].Slug
		clickName = clicks[n-1].Name
	}

	body, bodyColor, err := readThoughtBody("", me.PreferredBodyColor, false)
	if err != nil {
		return err
	}
	if err := api.ValidateThoughtBody(body); err != nil {
		return err
	}

	ui.Printf("\nPreview (%s):\n%s\n\n", scopeLabel(dest, me.Username, clickName), body)
	ok, err := ui.Confirm("Post this")
	if err != nil {
		return err
	}
	if !ok {
		ui.Println("Cancelled.")
		return nil
	}

	key := api.NewIdempotencyKey()
	var post api.Post
	switch dest {
	case "1":
		post, err = c.CreateGlobalPost(ctx, body, bodyColor, key)
	case "2":
		post, err = c.CreateWallPost(ctx, me.Username, body, bodyColor, key)
	case "3":
		post, err = c.CreateClickPost(ctx, clickSlug, body, bodyColor, key)
	}
	if err != nil {
		return formatErr(err)
	}
	ui.Printf("Posted · %s · %s\n", post.ID, post.CreatedAt)
	return nil
}

func showMine(ctx context.Context, interactive bool) error {
	c, err := client(true)
	if err != nil {
		return err
	}
	var all []api.Post
	pageNum := 1
	for {
		page, err := c.MyPosts(ctx, pageNum)
		if err != nil {
			return formatErr(err)
		}
		if pageNum == 1 && len(page.Posts) == 0 {
			ui.Println("No posts.")
			return nil
		}
		all = append(all, page.Posts...)
		if !interactive || !page.HasMore {
			break
		}
		more, err := ui.Confirm("Load more")
		if err != nil || !more {
			break
		}
		pageNum++
	}
	printMyPosts(all)
	if !interactive {
		return nil
	}
	choice, err := ui.ReadLine("Post # to view (Enter to skip): ")
	if err != nil || strings.TrimSpace(choice) == "" {
		return nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(choice))
	if err != nil || n < 1 || n > len(all) {
		return fmt.Errorf("pick a number between 1 and %d", len(all))
	}
	return showMyPostDetail(ctx, all[n-1])
}

func showMyPostDetail(ctx context.Context, post api.Post) error {
	ui.Printf("\n%s · %s · %s\n", postScopeLine(post), post.CreatedAt, post.ID)
	kind := postKindLabel(post)
	if kind != "" {
		ui.Printf("[%s]\n", kind)
	}
	if post.IsArt {
		ui.Printf("%s\n", ui.FormatArt(post.Body, post.ArtColors, false))
	} else {
		ui.Printf("%s\n", ui.Emojicon(post.Body))
	}
	if post.CanEdit || post.CanDelete {
		ui.Println()
		if post.CanEdit {
			ui.Println("e) Edit")
		}
		if post.CanDelete {
			ui.Println("d) Delete")
		}
		ui.Println("Enter) Back")
		choice, err := ui.ReadLine("> ")
		if err != nil {
			return err
		}
		switch strings.ToLower(strings.TrimSpace(choice)) {
		case "e":
			if !post.CanEdit {
				return fmt.Errorf("you cannot edit this post")
			}
			return editPost(ctx, post.ID)
		case "d":
			if !post.CanDelete {
				return fmt.Errorf("you cannot delete this post")
			}
			return deletePost(ctx, post.ID, true)
		}
	}
	return nil
}

func editPost(ctx context.Context, id string) error {
	c, err := client(true)
	if err != nil {
		return err
	}
	post, _, err := c.Post(ctx, id)
	if err != nil {
		return formatErr(err)
	}
	if !post.CanEdit {
		return fmt.Errorf("you cannot edit this post")
	}
	if post.IsArt || post.IsPoll {
		return fmt.Errorf("only plain thought posts can be edited from the CLI")
	}
	if post.EditRequiresReason {
		return fmt.Errorf("this edit requires a moderation reason — use the website")
	}
	body, bodyColor, colorChanged, err := readThoughtBodyForEdit(post.Body, post.BodyColor)
	if err != nil {
		return err
	}
	if err := api.ValidateThoughtBody(body); err != nil {
		return err
	}

	ui.Printf("\nPreview:\n%s\n\n", body)
	ok, err := ui.Confirm("Save this edit")
	if err != nil {
		return err
	}
	if !ok {
		ui.Println("Cancelled.")
		return nil
	}

	key := api.NewIdempotencyKey()
	var colorPtr *string
	if colorChanged {
		colorPtr = &bodyColor
	}
	updated, err := c.UpdatePost(ctx, id, body, colorPtr, "", key)
	if err != nil {
		return formatErr(err)
	}
	when := updated.UpdatedAt
	if when == "" {
		when = updated.CreatedAt
	}
	ui.Printf("Saved · %s\n", when)
	return nil
}

func deletePost(ctx context.Context, id string, confirm bool) error {
	c, err := client(true)
	if err != nil {
		return err
	}
	post, _, err := c.Post(ctx, id)
	if err != nil {
		return formatErr(err)
	}
	if !post.CanDelete {
		return fmt.Errorf("you cannot delete this post")
	}
	if post.DeleteRequiresReason {
		return fmt.Errorf("this delete requires a moderation reason — use the website")
	}
	if confirm {
		preview := ui.Truncate(ui.Emojicon(post.Body), 120)
		if post.IsArt {
			preview = ui.FormatArt(post.Body, post.ArtColors, true)
		}
		ui.Printf("Delete post %s?\n%s\n", id, preview)
		ok, err := ui.ReadLine("Type yes to confirm: ")
		if err != nil {
			return err
		}
		if strings.ToLower(strings.TrimSpace(ok)) != "yes" {
			ui.Println("Cancelled.")
			return nil
		}
	}
	key := api.NewIdempotencyKey()
	if err := c.DeletePost(ctx, id, "", key); err != nil {
		return formatErr(err)
	}
	ui.Println("Deleted.")
	return nil
}

func readThoughtBody(current, defaultColor string, keepEmptyColor bool) (body, bodyColor string, err error) {
	body, bodyColor, _, err = readThoughtBodyInner(current, defaultColor, keepEmptyColor)
	return body, bodyColor, err
}

func readThoughtBodyForEdit(currentBody, currentColor string) (body, bodyColor string, colorChanged bool, err error) {
	return readThoughtBodyInner(currentBody, currentColor, true)
}

func readThoughtBodyInner(current, defaultColor string, keepEmptyColor bool) (body, bodyColor string, colorChanged bool, err error) {
	if current != "" {
		ui.Printf("Current body (leave empty to keep):\n%s\n", current)
	}
	ui.Println("Thought (end with a line containing only .):")
	body, err = ui.ReadMultiline(".")
	if err != nil {
		return "", "", false, err
	}
	if body == "" && current != "" {
		body = current
	}
	promptColor := strings.TrimSpace(defaultColor)
	if promptColor == "" {
		promptColor = "default"
	}
	colorPrompt := fmt.Sprintf("Body color (default|blue|green|red|#rrggbb, Enter=%s): ", promptColor)
	colorIn, err := ui.ReadLine(colorPrompt)
	if err != nil {
		return "", "", false, err
	}
	colorIn = strings.TrimSpace(strings.ToLower(colorIn))
	if colorIn == "" {
		if keepEmptyColor {
			return body, promptColor, false, nil
		}
		return body, promptColor, true, nil
	}
	if !validBodyColor(colorIn) {
		return "", "", false, fmt.Errorf("color must be default, blue, green, red, or #rrggbb")
	}
	return body, colorIn, true, nil
}

func validBodyColor(c string) bool {
	switch c {
	case "default", "blue", "green", "red":
		return true
	}
	if len(c) == 4 && c[0] == '#' {
		return isHex(c[1:])
	}
	if len(c) == 7 && c[0] == '#' {
		return isHex(c[1:])
	}
	return false
}

func isHex(s string) bool {
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func printMyPosts(posts []api.Post) {
	for i, p := range posts {
		if i > 0 {
			ui.Println()
			ui.Println("---")
		}
		kind := postKindLabel(p)
		prefix := ""
		if kind != "" {
			prefix = kind + " · "
		}
		snippet := ui.Truncate(ui.Emojicon(p.Body), 120)
		if p.IsArt {
			snippet = ui.FormatArt(p.Body, p.ArtColors, true)
		}
		ui.Printf("%2d. %s%s · %s · %d comments · %s\n    %s\n",
			i+1, prefix, postScopeLine(p), p.CreatedAt, p.CommentCount, p.ID, snippet)
	}
}

func postScopeLine(p api.Post) string {
	switch p.Scope {
	case "click":
		if p.ClickName != "" {
			return "Click: " + p.ClickName
		}
		if p.ClickSlug != "" {
			return "Click: " + p.ClickSlug
		}
		return "Click"
	case "profile":
		return "Profile wall"
	default:
		return "Global feed"
	}
}

func postKindLabel(p api.Post) string {
	if p.IsArt {
		return "art"
	}
	if p.IsPoll {
		return "poll"
	}
	return ""
}

func scopeLabel(choice, username, clickName string) string {
	switch choice {
	case "1":
		return "global feed"
	case "2":
		return "@" + username + " wall"
	case "3":
		if clickName != "" {
			return "Click: " + clickName
		}
		return "Click"
	default:
		return "post"
	}
}
