package app

import (
	"context"
	"fmt"
	"html"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"komari-bot/internal/config"
	"komari-bot/internal/currency"
	"komari-bot/internal/komari"
	"komari-bot/internal/store"
)

const (
	callbackMenuAdd        = "menu:add"
	callbackMenuLatency    = "menu:latency"
	callbackMenuReminder   = "menu:reminder"
	callbackBackMenu       = "nav:menu"
	callbackAddRefresh     = "add:refresh"
	callbackAddConfirm     = "add:confirm"
	callbackLatencyRefresh = "latency:refresh"
)

type App struct {
	cfg        config.Config
	store      *store.Store
	komari     *komari.Client
	converter  *currency.Converter
	bot        *tgbotapi.BotAPI
	location   *time.Location
	sessionMu  sync.Mutex
	addSession map[int64]map[string]bool
}

func New(cfg config.Config, db *store.Store, komariClient *komari.Client, converter *currency.Converter) (*App, error) {
	bot, err := tgbotapi.NewBotAPI(cfg.TelegramBotToken)
	if err != nil {
		return nil, err
	}

	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return nil, err
	}

	return &App{
		cfg:        cfg,
		store:      db,
		komari:     komariClient,
		converter:  converter,
		bot:        bot,
		location:   loc,
		addSession: map[int64]map[string]bool{},
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	go a.reminderLoop(ctx)

	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 30
	updates := a.bot.GetUpdatesChan(updateConfig)

	for {
		select {
		case <-ctx.Done():
			a.bot.StopReceivingUpdates()
			return nil
		case update, ok := <-updates:
			if !ok {
				return nil
			}
			if err := a.handleUpdate(update); err != nil {
				log.Printf("handle update: %v", err)
			}
		}
	}
}

func (a *App) handleUpdate(update tgbotapi.Update) error {
	switch {
	case update.Message != nil:
		return a.handleMessage(update.Message)
	case update.CallbackQuery != nil:
		return a.handleCallback(update.CallbackQuery)
	default:
		return nil
	}
}

func (a *App) handleMessage(message *tgbotapi.Message) error {
	if message.From == nil || !message.IsCommand() {
		return nil
	}

	switch message.Command() {
	case "start":
		if !a.isAdmin(message.From.ID) {
			return a.reply(message.Chat.ID, "该机器人只接受已配置管理员操作。")
		}
		return a.replyWithMarkup(message.Chat.ID, "输入 /admin 打开管理面板。", adminMenuMarkup())
	case "admin":
		if !a.isAdmin(message.From.ID) {
			return a.reply(message.Chat.ID, "无权限。")
		}
		return a.replyWithMarkup(message.Chat.ID, "管理面板", adminMenuMarkup())
	default:
		return nil
	}
}

func (a *App) handleCallback(query *tgbotapi.CallbackQuery) error {
	if query.From == nil || query.Message == nil {
		return nil
	}
	if !a.isAdmin(query.From.ID) {
		return a.answerCallback(query.ID, "无权限")
	}

	data := query.Data
	switch {
	case data == callbackMenuAdd:
		if err := a.answerCallback(query.ID, "刷新可添加服务器"); err != nil {
			return err
		}
		return a.showAddServerView(query.Message.Chat.ID, query.Message.MessageID, query.From.ID)
	case data == callbackMenuLatency:
		if err := a.answerCallback(query.ID, "加载延迟监测"); err != nil {
			return err
		}
		return a.showLatencyServerList(query.Message.Chat.ID, query.Message.MessageID)
	case data == callbackMenuReminder:
		if err := a.answerCallback(query.ID, "执行提醒检查"); err != nil {
			return err
		}
		sent, err := a.runReminderCheck(true)
		if err != nil {
			return a.editMessage(query.Message.Chat.ID, query.Message.MessageID, "提醒检查失败："+html.EscapeString(err.Error()), adminMenuMarkup())
		}
		text := fmt.Sprintf("提醒检查完成，本次发送 %d 条提醒。", sent)
		return a.editMessage(query.Message.Chat.ID, query.Message.MessageID, text, adminMenuMarkup())
	case data == callbackBackMenu:
		if err := a.answerCallback(query.ID, "返回菜单"); err != nil {
			return err
		}
		return a.editMessage(query.Message.Chat.ID, query.Message.MessageID, "管理面板", adminMenuMarkup())
	case data == callbackAddRefresh:
		if err := a.answerCallback(query.ID, "已刷新"); err != nil {
			return err
		}
		return a.showAddServerView(query.Message.Chat.ID, query.Message.MessageID, query.From.ID)
	case data == callbackAddConfirm:
		if err := a.answerCallback(query.ID, "确认添加"); err != nil {
			return err
		}
		return a.confirmAddServers(query.Message.Chat.ID, query.Message.MessageID, query.From.ID)
	case strings.HasPrefix(data, "add:toggle:"):
		if err := a.answerCallback(query.ID, "已切换"); err != nil {
			return err
		}
		uuid := strings.TrimPrefix(data, "add:toggle:")
		a.toggleAddSelection(query.From.ID, uuid)
		return a.showAddServerView(query.Message.Chat.ID, query.Message.MessageID, query.From.ID)
	case data == callbackLatencyRefresh:
		if err := a.answerCallback(query.ID, "已刷新"); err != nil {
			return err
		}
		return a.showLatencyServerList(query.Message.Chat.ID, query.Message.MessageID)
	case strings.HasPrefix(data, "latency:view:"):
		if err := a.answerCallback(query.ID, "加载延迟数据"); err != nil {
			return err
		}
		uuid := strings.TrimPrefix(data, "latency:view:")
		return a.showLatencyDetail(query.Message.Chat.ID, query.Message.MessageID, uuid)
	case strings.HasPrefix(data, "paid:"):
		parts := strings.SplitN(data, ":", 3)
		if len(parts) != 3 {
			return a.answerCallback(query.ID, "按钮数据无效")
		}
		acked, err := a.store.AcknowledgeReminder(parts[1], parts[2])
		if err != nil {
			return err
		}
		if !acked {
			return a.answerCallback(query.ID, "该提醒已经失效或已更新")
		}
		if err := a.answerCallback(query.ID, "已标记为已续费"); err != nil {
			return err
		}
		text := query.Message.Text + "\n\n已标记为已续费，本轮到期将不再提醒。"
		return a.editMessage(query.Message.Chat.ID, query.Message.MessageID, text, nil)
	default:
		return a.answerCallback(query.ID, "未识别的操作")
	}
}

func (a *App) showAddServerView(chatID int64, messageID int, userID int64) error {
	nodes, err := a.komari.FetchNodes()
	if err != nil {
		return a.editMessage(chatID, messageID, "读取 Komari 节点失败："+html.EscapeString(err.Error()), backOnlyMarkup())
	}

	managedMap, err := a.store.ManagedServerMap()
	if err != nil {
		return err
	}

	selection := a.selectionForUser(userID)
	filtered := make([]komari.Node, 0)
	valid := map[string]komari.Node{}
	for _, node := range nodes {
		if _, exists := managedMap[node.UUID]; exists {
			continue
		}
		filtered = append(filtered, node)
		valid[node.UUID] = node
	}

	a.sessionMu.Lock()
	for uuid := range selection {
		if _, ok := valid[uuid]; !ok {
			delete(selection, uuid)
		}
	}
	a.sessionMu.Unlock()

	text := "添加服务器\n\n选择要纳入 bot 管理的服务器，然后点击底部确认。再次进入本页会按 Komari 最新数据刷新。"
	if len(filtered) == 0 {
		text = "没有可添加的服务器，Komari 中当前所有节点都已加入 bot。"
	}

	return a.editMessage(chatID, messageID, text, a.addServerMarkup(filtered, selection))
}

func (a *App) confirmAddServers(chatID int64, messageID int, userID int64) error {
	nodes, err := a.komari.FetchNodes()
	if err != nil {
		return a.editMessage(chatID, messageID, "读取 Komari 节点失败："+html.EscapeString(err.Error()), backOnlyMarkup())
	}

	managedMap, err := a.store.ManagedServerMap()
	if err != nil {
		return err
	}

	selection := a.selectionForUser(userID)
	var toAdd []store.ManagedServer
	for _, node := range nodes {
		if !selection[node.UUID] {
			continue
		}
		if _, exists := managedMap[node.UUID]; exists {
			continue
		}
		toAdd = append(toAdd, store.ManagedServer{
			ServerUUID: node.UUID,
			ServerName: node.Name,
		})
	}

	if len(toAdd) == 0 {
		return a.editMessage(chatID, messageID, "没有可确认的服务器，请先勾选节点。", backOnlyMarkup())
	}

	if err := a.store.AddManagedServers(toAdd); err != nil {
		return err
	}

	a.clearSelection(userID)

	names := make([]string, 0, len(toAdd))
	for _, server := range toAdd {
		names = append(names, server.ServerName)
	}

	text := "已添加服务器：\n- " + strings.Join(names, "\n- ")
	return a.editMessage(chatID, messageID, text, adminMenuMarkup())
}

func (a *App) showLatencyServerList(chatID int64, messageID int) error {
	servers, err := a.store.ListManagedServers()
	if err != nil {
		return err
	}

	if len(servers) == 0 {
		return a.editMessage(chatID, messageID, "还没有已添加服务器，请先在“添加服务器”中选择节点。", backOnlyMarkup())
	}

	text := "延迟监测\n\n点击服务器查看最近的 Ping 聚合数据。"
	return a.editMessage(chatID, messageID, text, latencyServerMarkup(servers))
}

func (a *App) showLatencyDetail(chatID int64, messageID int, uuid string) error {
	managedMap, err := a.store.ManagedServerMap()
	if err != nil {
		return err
	}

	server, ok := managedMap[uuid]
	if !ok {
		return a.editMessage(chatID, messageID, "该服务器未加入 bot 管理。", backOnlyMarkup())
	}

	data, err := a.komari.FetchPingData(uuid, a.cfg.PingHours)
	if err != nil {
		return a.editMessage(chatID, messageID, "读取延迟数据失败："+html.EscapeString(err.Error()), latencyDetailBackMarkup())
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("服务器：<b>%s</b>\n", html.EscapeString(server.ServerName)))
	builder.WriteString(fmt.Sprintf("统计范围：最近 %d 小时\n", a.cfg.PingHours))

	if len(data.Tasks) == 0 {
		builder.WriteString("\n暂无 Ping 监测数据。")
		return a.editMessage(chatID, messageID, builder.String(), latencyDetailBackMarkup())
	}

	sort.Slice(data.Tasks, func(i, j int) bool {
		return data.Tasks[i].Name < data.Tasks[j].Name
	})

	for _, task := range data.Tasks {
		builder.WriteString("\n")
		builder.WriteString(fmt.Sprintf("• <b>%s</b> (%s)\n", html.EscapeString(task.Name), html.EscapeString(task.Type)))
		builder.WriteString(fmt.Sprintf("  平均 %.2fms | 最低 %.2fms | 最高 %.2fms | 丢包 %.2f%% | 样本 %d\n",
			task.Avg, task.Min, task.Max, task.Loss, task.Total))
	}

	if len(data.BasicInfo) > 0 {
		builder.WriteString("\n按探针节点汇总：")
		for _, item := range data.BasicInfo {
			builder.WriteString(fmt.Sprintf("\n• %s | 丢包 %.2f%% | 最低 %.2fms | 最高 %.2fms",
				shortUUID(item.Client), item.Loss, item.Min, item.Max))
		}
	}

	return a.editMessage(chatID, messageID, builder.String(), latencyDetailBackMarkup())
}

func (a *App) reminderLoop(ctx context.Context) {
	if _, err := a.runReminderCheck(false); err != nil {
		log.Printf("initial reminder check: %v", err)
	}

	ticker := time.NewTicker(a.cfg.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := a.runReminderCheck(false); err != nil {
				log.Printf("reminder check: %v", err)
			}
		}
	}
}

func (a *App) runReminderCheck(force bool) (int, error) {
	nodes, err := a.komari.FetchNodes()
	if err != nil {
		return 0, err
	}

	managedMap, err := a.store.ManagedServerMap()
	if err != nil {
		return 0, err
	}

	var updates []store.ManagedServer
	nodeMap := make(map[string]komari.Node, len(nodes))
	for _, node := range nodes {
		nodeMap[node.UUID] = node
		if managed, ok := managedMap[node.UUID]; ok && managed.ServerName != node.Name {
			updates = append(updates, store.ManagedServer{ServerUUID: node.UUID, ServerName: node.Name})
		}
	}
	if err := a.store.UpdateManagedServers(updates); err != nil {
		return 0, err
	}

	now := time.Now().In(a.location)
	today := now.Format("2006-01-02")
	sent := 0

	for uuid, managed := range managedMap {
		node, ok := nodeMap[uuid]
		if !ok {
			continue
		}

		expiry, cycleKey, daysRemaining, remindable, err := a.reminderWindow(node, now)
		if err != nil || !remindable {
			continue
		}

		state, exists, err := a.store.GetReminderState(uuid)
		if err != nil {
			return sent, err
		}
		if !exists || state.CycleKey != cycleKey {
			state = store.ReminderState{
				ServerUUID:     uuid,
				CycleKey:       cycleKey,
				Acknowledged:   false,
				LastNotifiedOn: "",
			}
		}
		if state.Acknowledged {
			continue
		}
		if !force && state.LastNotifiedOn == today {
			continue
		}

		text := a.buildReminderText(managed.ServerName, node, expiry, daysRemaining)
		markup := paidMarkup(uuid, cycleKey)
		for _, chatID := range a.cfg.TelegramNotifyChatIDs {
			message := tgbotapi.NewMessage(chatID, text)
			message.ParseMode = tgbotapi.ModeHTML
			message.ReplyMarkup = markup
			if _, err := a.bot.Send(message); err != nil {
				return sent, err
			}
		}

		state.LastNotifiedOn = today
		if err := a.store.SaveReminderState(state); err != nil {
			return sent, err
		}
		sent++
	}

	return sent, nil
}

func (a *App) reminderWindow(node komari.Node, now time.Time) (time.Time, string, int, bool, error) {
	expiry, err := parseKomariTime(node.ExpiredAt)
	if err != nil {
		return time.Time{}, "", 0, false, err
	}
	if expiry.IsZero() || expiry.Year() <= 1 {
		return time.Time{}, "", 0, false, nil
	}

	localExpiry := expiry.In(a.location)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, a.location)
	expiryStart := time.Date(localExpiry.Year(), localExpiry.Month(), localExpiry.Day(), 0, 0, 0, 0, a.location)
	daysRemaining := int(expiryStart.Sub(todayStart).Hours() / 24)
	if daysRemaining < 0 || daysRemaining > a.cfg.ReminderDays {
		return localExpiry, localExpiry.Format("2006-01-02"), daysRemaining, false, nil
	}

	return localExpiry, localExpiry.Format("2006-01-02"), daysRemaining, true, nil
}

func (a *App) buildReminderText(serverName string, node komari.Node, expiry time.Time, daysRemaining int) string {
	amount := a.converter.FormatCNY(node.Price, node.Currency)
	autoRenewal := "否"
	if node.AutoRenewal {
		autoRenewal = "是"
	}

	return fmt.Sprintf(
		"<b>%s</b> 服务器剩余 <b>%d</b> 日到期\n续费金额 %s\n到期时间 %s\n计费周期 %d 天\n自动续费 %s",
		html.EscapeString(serverName),
		daysRemaining,
		html.EscapeString(amount),
		expiry.Format("2006-01-02 15:04:05"),
		node.BillingCycle,
		autoRenewal,
	)
}

func adminMenuMarkup() *tgbotapi.InlineKeyboardMarkup {
	markup := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("添加服务器", callbackMenuAdd),
			tgbotapi.NewInlineKeyboardButtonData("延迟监测", callbackMenuLatency),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("立即检查提醒", callbackMenuReminder),
		),
	)
	return &markup
}

func (a *App) addServerMarkup(nodes []komari.Node, selection map[string]bool) *tgbotapi.InlineKeyboardMarkup {
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(nodes)+2)
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Name < nodes[j].Name
	})

	for _, node := range nodes {
		prefix := "☐"
		if selection[node.UUID] {
			prefix = "☑"
		}
		label := fmt.Sprintf("%s %s", prefix, node.Name)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, "add:toggle:"+node.UUID),
		))
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("刷新列表", callbackAddRefresh),
	))
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("确认添加", callbackAddConfirm),
		tgbotapi.NewInlineKeyboardButtonData("返回菜单", callbackBackMenu),
	))

	markup := tgbotapi.NewInlineKeyboardMarkup(rows...)
	return &markup
}

func latencyServerMarkup(servers []store.ManagedServer) *tgbotapi.InlineKeyboardMarkup {
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(servers)+2)
	for _, server := range servers {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(server.ServerName, "latency:view:"+server.ServerUUID),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("刷新列表", callbackLatencyRefresh),
	))
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("返回菜单", callbackBackMenu),
	))

	markup := tgbotapi.NewInlineKeyboardMarkup(rows...)
	return &markup
}

func latencyDetailBackMarkup() *tgbotapi.InlineKeyboardMarkup {
	markup := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("返回服务器列表", callbackMenuLatency),
			tgbotapi.NewInlineKeyboardButtonData("返回菜单", callbackBackMenu),
		),
	)
	return &markup
}

func backOnlyMarkup() *tgbotapi.InlineKeyboardMarkup {
	markup := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("返回菜单", callbackBackMenu),
		),
	)
	return &markup
}

func paidMarkup(serverUUID, cycleKey string) *tgbotapi.InlineKeyboardMarkup {
	markup := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("已续费", "paid:"+serverUUID+":"+cycleKey),
		),
	)
	return &markup
}

func (a *App) editMessage(chatID int64, messageID int, text string, markup *tgbotapi.InlineKeyboardMarkup) error {
	edit := tgbotapi.NewEditMessageText(chatID, messageID, text)
	edit.ParseMode = tgbotapi.ModeHTML
	if markup != nil {
		edit.ReplyMarkup = markup
	}
	_, err := a.bot.Send(edit)
	return err
}

func (a *App) reply(chatID int64, text string) error {
	message := tgbotapi.NewMessage(chatID, text)
	_, err := a.bot.Send(message)
	return err
}

func (a *App) replyWithMarkup(chatID int64, text string, markup *tgbotapi.InlineKeyboardMarkup) error {
	message := tgbotapi.NewMessage(chatID, text)
	message.ReplyMarkup = markup
	message.ParseMode = tgbotapi.ModeHTML
	_, err := a.bot.Send(message)
	return err
}

func (a *App) answerCallback(callbackID, text string) error {
	_, err := a.bot.Request(tgbotapi.NewCallback(callbackID, text))
	return err
}

func (a *App) isAdmin(userID int64) bool {
	for _, id := range a.cfg.TelegramAdminIDs {
		if id == userID {
			return true
		}
	}
	return false
}

func (a *App) selectionForUser(userID int64) map[string]bool {
	a.sessionMu.Lock()
	defer a.sessionMu.Unlock()

	selection, ok := a.addSession[userID]
	if !ok {
		selection = map[string]bool{}
		a.addSession[userID] = selection
	}
	copied := make(map[string]bool, len(selection))
	for uuid, selected := range selection {
		copied[uuid] = selected
	}
	return copied
}

func (a *App) toggleAddSelection(userID int64, uuid string) {
	a.sessionMu.Lock()
	defer a.sessionMu.Unlock()

	selection, ok := a.addSession[userID]
	if !ok {
		selection = map[string]bool{}
		a.addSession[userID] = selection
	}
	if selection[uuid] {
		delete(selection, uuid)
		return
	}
	selection[uuid] = true
}

func (a *App) clearSelection(userID int64) {
	a.sessionMu.Lock()
	defer a.sessionMu.Unlock()
	delete(a.addSession, userID)
}

func parseKomariTime(raw string) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" || strings.HasPrefix(value, "0001-") {
		return time.Time{}, nil
	}

	layouts := []string{
		"2006-01-02 15:04:05.9999999Z07:00",
		"2006-01-02 15:04:05.999999999Z07:00",
		time.RFC3339Nano,
		time.RFC3339,
	}

	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, nil
		}
	}

	return time.Time{}, fmt.Errorf("unsupported time format: %s", raw)
}

func shortUUID(uuid string) string {
	if len(uuid) <= 8 {
		return uuid
	}
	return uuid[:8]
}
