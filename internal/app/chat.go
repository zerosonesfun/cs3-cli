package app

import (
	"context"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/zerosonesfun/cs3-cli/internal/api"
	"github.com/zerosonesfun/cs3-cli/internal/ui"
)

func cmdChat() *cobra.Command {
	return &cobra.Command{
		Use:   "chat",
		Short: "Join the scheduled chatroom when it is open",
		Long: `Enter the site chatroom when a scheduled session is live.

If the room is closed, upcoming session times are shown in your local timezone.
When it is open you are asked once per session whether to erase your messages on logout,
then you join and can send and receive messages until you leave (b / q).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runChat(cmd.Context())
		},
	}
}

func runChat(ctx context.Context) error {
	c, err := client(true)
	if err != nil {
		return err
	}
	state, err := c.ChatState(ctx)
	if err != nil {
		return formatErr(err)
	}

	if !state.Live {
		ui.Println("The chatroom is not open right now.")
		printChatSchedule(state.Schedule)
		return nil
	}
	if state.Error != "" && !state.CanJoin && !state.Present {
		ui.Println(state.Error)
		return nil
	}
	if state.Full && !state.Present {
		ui.Println("The chatroom is full. Check back later — someone may leave.")
		return nil
	}
	if !state.CanJoin && !state.Present {
		if state.Error != "" {
			ui.Println(state.Error)
		} else {
			ui.Println("You cannot join the chatroom right now.")
		}
		return nil
	}

	session := strings.TrimSpace(state.SessionStartsAt)
	answered := session != "" && session == strings.TrimSpace(state.Prefs.ChatErasePromptSession)
	if !answered && session != "" {
		ui.Println("Erase all of your chat messages when you log out of the site?")
		ui.Println("If yes, your messages become anonymous placeholders when you log out.")
		erase, err := ui.Confirm("Yes, erase on logout")
		if err != nil {
			return err
		}
		if err := c.ChatPrefs(ctx, &erase, ""); err != nil {
			return formatErr(err)
		}
		if erase {
			ui.Println("Your chat messages will be erased when you log out.")
		} else {
			ui.Println("Your chat messages will be kept when you log out.")
		}
	}

	presenceCount, err := c.ChatPresence(ctx)
	if err != nil {
		return formatErr(err)
	}
	var leaveOnce sync.Once
	leave := func() {
		leaveOnce.Do(func() {
			_ = c.ChatPresenceLeave(context.Background())
		})
	}
	defer leave()

	bodyColor := strings.TrimSpace(state.Prefs.ChatBodyColor)
	if bodyColor == "" {
		bodyColor = "default"
	}
	maxBody := state.MaxBody
	if maxBody < 1 {
		maxBody = 1000
	}
	isMod := state.IsMod

	ui.Printf("Joined chat (%d here). Type a message and Enter. b or q leaves.\n", presenceCount)
	ui.Println("Commands: here · /color … · d <id> · help")
	if isMod {
		ui.Println("Mod: ban <user_id> <1h|24h|7d|permanent> <reason> · unban <user_id>")
	}
	ui.Println()

	var (
		mu      sync.Mutex
		afterID int
		seen    = map[int]bool{}
	)
	for _, m := range state.Messages {
		printChatMessage(m)
		seen[m.ID] = true
		if m.ID > afterID {
			afterID = m.ID
		}
	}

	roomCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		select {
		case <-sigCh:
			ui.Println("\nLeaving chat.")
			leave()
			cancel()
			ui.Println("Press Enter to continue.")
		case <-roomCtx.Done():
		}
	}()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for {
			select {
			case <-roomCtx.Done():
				return
			default:
			}
			mu.Lock()
			pollAfter := afterID
			mu.Unlock()
			msgs, _, err := c.ChatMessages(roomCtx, pollAfter)
			if roomCtx.Err() != nil {
				return
			}
			if err != nil {
				if isUnauthorized(err) {
					cancel()
					return
				}
				mu.Lock()
				ui.Printf("\n(poll error: %v)\n", err)
				mu.Unlock()
				select {
				case <-roomCtx.Done():
					return
				case <-time.After(2 * time.Second):
				}
				continue
			}
			if len(msgs) == 0 {
				continue
			}
			mu.Lock()
			for _, m := range msgs {
				if seen[m.ID] {
					continue
				}
				seen[m.ID] = true
				printChatMessage(m)
				if m.ID > afterID {
					afterID = m.ID
				}
			}
			mu.Unlock()
		}
	}()

	go func() {
		defer wg.Done()
		t := time.NewTicker(45 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-roomCtx.Done():
				return
			case <-t.C:
				if _, err := c.ChatPresence(roomCtx); err != nil {
					if isUnauthorized(err) {
						cancel()
						return
					}
				}
			}
		}
	}()

	for {
		if roomCtx.Err() != nil {
			wg.Wait()
			return nil
		}
		line, err := ui.ReadLine("> ")
		if err != nil {
			cancel()
			wg.Wait()
			if roomCtx.Err() != nil {
				return nil
			}
			return err
		}
		if roomCtx.Err() != nil {
			wg.Wait()
			return nil
		}
		trim := strings.TrimSpace(line)
		lower := strings.ToLower(trim)

		if lower == "" {
			continue
		}
		if lower == "b" || lower == "q" || lower == "quit" || lower == "leave" || lower == "0" {
			ui.Println("Leaving chat.")
			cancel()
			wg.Wait()
			return nil
		}
		if lower == "help" || lower == "h" || lower == "?" {
			printChatRoomHelp(isMod)
			continue
		}
		if lower == "here" {
			users, err := c.ChatPresenceList(roomCtx)
			if err != nil {
				ui.Printf("Error: %v\n", formatErr(err))
				continue
			}
			if len(users) == 0 {
				ui.Println("No one listed right now.")
				continue
			}
			ui.Printf("Here (%d):\n", len(users))
			for _, u := range users {
				if isMod {
					ui.Printf("  %d @%s\n", u.ID, u.Username)
				} else {
					ui.Printf("  @%s\n", u.Username)
				}
			}
			continue
		}
		if strings.HasPrefix(lower, "/color") {
			colorIn := strings.TrimSpace(trim[len("/color"):])
			colorIn = strings.TrimSpace(strings.ToLower(colorIn))
			if colorIn == "" {
				ui.Printf("Current color: %s\n", bodyColor)
				ui.Println("Usage: /color default|blue|green|red|#rrggbb")
				continue
			}
			if !validBodyColor(colorIn) {
				ui.Println("Color must be default, blue, green, red, or #rrggbb.")
				continue
			}
			if err := c.ChatPrefs(roomCtx, nil, colorIn); err != nil {
				ui.Printf("Error: %v\n", formatErr(err))
				continue
			}
			bodyColor = colorIn
			ui.Printf("Chat color set to %s.\n", bodyColor)
			continue
		}
		if strings.HasPrefix(lower, "d ") || lower == "d" {
			parts := strings.Fields(trim)
			if len(parts) < 2 {
				ui.Println("Usage: d <message_id>")
				continue
			}
			id, err := strconv.Atoi(parts[1])
			if err != nil || id < 1 {
				ui.Println("Usage: d <message_id>")
				continue
			}
			reason := ""
			if isMod && len(parts) >= 3 {
				reason = strings.TrimSpace(strings.Join(parts[2:], " "))
			}
			updated, err := c.ChatDeleteMessage(roomCtx, id, reason)
			if err != nil {
				ui.Printf("Error: %v\n", formatErr(err))
				continue
			}
			if updated.ID > 0 {
				mu.Lock()
				printChatMessage(updated)
				mu.Unlock()
			} else {
				ui.Println("Deleted.")
			}
			continue
		}
		if isMod && strings.HasPrefix(lower, "ban ") {
			parts := strings.Fields(trim)
			if len(parts) < 4 {
				ui.Println("Usage: ban <user_id> <1h|24h|7d|permanent> <reason>")
				continue
			}
			uid, err := strconv.Atoi(parts[1])
			if err != nil || uid < 1 {
				ui.Println("Usage: ban <user_id> <1h|24h|7d|permanent> <reason>")
				continue
			}
			dur := strings.ToLower(parts[2])
			switch dur {
			case "1h", "24h", "7d", "permanent":
			default:
				ui.Println("Duration must be 1h, 24h, 7d, or permanent.")
				continue
			}
			reason := strings.TrimSpace(strings.Join(parts[3:], " "))
			if err := c.ChatBan(roomCtx, uid, reason, dur); err != nil {
				ui.Printf("Error: %v\n", formatErr(err))
				continue
			}
			ui.Println("Banned.")
			continue
		}
		if isMod && strings.HasPrefix(lower, "unban ") {
			parts := strings.Fields(trim)
			if len(parts) != 2 {
				ui.Println("Usage: unban <user_id>")
				continue
			}
			uid, err := strconv.Atoi(parts[1])
			if err != nil || uid < 1 {
				ui.Println("Usage: unban <user_id>")
				continue
			}
			if err := c.ChatUnban(roomCtx, uid); err != nil {
				ui.Printf("Error: %v\n", formatErr(err))
				continue
			}
			ui.Println("Unbanned.")
			continue
		}

		if utf8.RuneCountInString(trim) > maxBody {
			ui.Printf("Message is too long (max %d characters).\n", maxBody)
			continue
		}
		msg, err := c.ChatSend(roomCtx, trim, bodyColor)
		if err != nil {
			ui.Printf("Error: %v\n", formatErr(err))
			if isUnauthorized(err) {
				cancel()
				wg.Wait()
				return err
			}
			continue
		}
		mu.Lock()
		if msg.ID > 0 && !seen[msg.ID] {
			seen[msg.ID] = true
			printChatMessage(msg)
			if msg.ID > afterID {
				afterID = msg.ID
			}
		}
		mu.Unlock()
	}
}

func printChatRoomHelp(isMod bool) {
	ui.Println("Type a message and press Enter.")
	ui.Println("  b / q     leave")
	ui.Println("  here      who is here")
	ui.Println("  /color …  set sticky body color")
	ui.Println("  d <id>    delete your message (mods: d <id> <reason>)")
	ui.Println("  help      this help")
	if isMod {
		ui.Println("  ban <user_id> <1h|24h|7d|permanent> <reason>")
		ui.Println("  unban <user_id>")
	}
}

func printChatMessage(m api.ChatMessage) {
	name := strings.TrimSpace(m.Username)
	if name == "" {
		name = "user"
	}
	prefix := "@" + name
	ago := strings.TrimSpace(m.TimeAgo)
	if ago != "" && !strings.EqualFold(ago, "0s ago") {
		prefix += " [" + ago + "]"
	}
	if m.ID > 0 {
		ui.Printf("#%d %s: %s\n", m.ID, prefix, m.Body)
	} else {
		ui.Printf("%s: %s\n", prefix, m.Body)
	}
}

func printChatSchedule(slots []api.ChatScheduleSlot) {
	if len(slots) == 0 {
		ui.Println("No upcoming sessions are scheduled.")
		return
	}
	ui.Println("Upcoming sessions:")
	for _, s := range slots {
		label := chatSlotLocalLabel(s.StartsAt, s.EndsAt)
		if label == "" {
			label = strings.TrimSpace(s.Label)
		}
		if label == "" {
			continue
		}
		ui.Printf("  · %s\n", label)
	}
}

func chatSlotLocalLabel(startsAt, endsAt string) string {
	startsAt = strings.TrimSpace(startsAt)
	endsAt = strings.TrimSpace(endsAt)
	if startsAt == "" || endsAt == "" {
		return ""
	}
	const layout = "2006-01-02 15:04:05"
	start, err1 := time.ParseInLocation(layout, startsAt, time.UTC)
	end, err2 := time.ParseInLocation(layout, endsAt, time.UTC)
	if err1 != nil || err2 != nil {
		return ""
	}
	local := time.Local
	start = start.In(local)
	end = end.In(local)
	fmtStart := start.Format("Jan 2, 2006 3:04 PM MST")
	fmtEnd := end.Format("Jan 2, 2006 3:04 PM MST")
	return fmtStart + " – " + fmtEnd
}
