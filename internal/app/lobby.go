package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"preferans/config"
	"strings"
	"time"

	"preferans/internal/p2p"
	"preferans/internal/rooms"
)

// lobbyPeriod — период опроса и объявления в публичном лобби.
var lobbyPeriod = 60 * time.Second

// Объявления идут через публичный ntfy.sh, у которого есть суточный лимит сообщений на IP-адрес (HTTP 429). Устройства
// одной семьи сидят за одним адресом, поэтому объявляемся редко: раз в 5 периодов (и сразу при смене адресов relay).
// Живость ведущего при этом проверяет не свежесть объявления, а попытка обмена каталогами (см. lobbyFail).
func lobbyAnnounceEvery() time.Duration { return 5 * lobbyPeriod }

// lobbyAlive — как долго объявление считается живым при выборе ведущего.
func lobbyAlive() time.Duration { return 12 * lobbyPeriod }

// lobbyBlocking — объявление моложе этого срока и недоступное устройству считается живым игроком, которого мы не достаём
// (до выяснения комнаты не создаются).
func lobbyBlocking() time.Duration { return 6 * lobbyPeriod }

// lobbyReadWindow — глубина чтения темы лобби.
func lobbyReadWindow() time.Duration { return lobbyAlive() + lobbyPeriod }

// lobbyBlockPause — пауза в объявлениях после ответа ntfy 429 (лимит сообщений исчерпан).
const lobbyBlockPause = 5 * time.Minute

// lobbyGatherLimit — дольше этого срока после запуска «сбор игроков» не показывается, даже если лобби так и не определилось
// (например, ntfy недоступен): тогда комната работает на своём сервисе, как раньше.
const lobbyGatherLimit = 40 * time.Second

// gatheringLocked: идёт ли сбор игроков в комнате — лобби ещё не определило общий сервис комнат либо часть живых игроков
// пока недоступна. Пока так, создавать и восстанавливать стол нельзя: стол попал бы на сервис, которого другие не видят.
func (a *App) gatheringLocked() bool {
	cur := a.rooms.config.Current
	if !a.lobbyEnabled || a.appearance.RoomServiceURL != "" || cur == "" || cur == localRoomID {
		return false
	}
	if a.lobbySkipWait {
		return false
	}
	if a.lobbyUnsettled {
		return true
	}
	return !a.lobbyReady && time.Since(a.lobbyGatherSince) < lobbyGatherLimit
}

func (a *App) setLobbyReady() {
	a.mu.Lock()
	changed := !a.lobbyReady
	a.lobbyReady = true
	a.mu.Unlock()
	if changed {
		a.lobbyLog("лобби: сбор игроков завершён")
	}
}

// roomWaitLocked описывает ожидание, которое видит игрок на экране комнаты: «gathering» — идёт сбор игроков,
// «offline» — нет связи с сервисом комнат; вторым значением — сколько секунд оно длится.
func (a *App) roomWaitLocked(gathering, online bool) (string, int) {
	if a.rooms.config.Current == "" || a.rooms.config.Current == localRoomID {
		return "", 0
	}
	var kind string
	var since time.Time
	switch {
	case gathering:
		kind, since = "gathering", a.lobbyGatherSince
		if a.lobbyUnsettled && !a.lobbyUnsettledSince.IsZero() {
			since = a.lobbyUnsettledSince
		}
	case !online:
		kind, since = "offline", a.rooms.seen
		if since.IsZero() {
			since = a.lobbyStart
		}
	default:
		return "", 0
	}
	if since.IsZero() {
		return kind, 0
	}
	return kind, max(0, int(time.Since(since).Seconds()))
}

// SkipRoomWait прерывает ожидание сбора игроков и связи с сервисом комнат: игрок знает, что договорился с остальными
// о нужной комнате другим способом. Стол можно создать сразу, даже если часть живых игроков этому устройству недоступна
// (пока не сменится комната или все не станут доступны). Если сервис комнат не отвечает, это устройство берёт сервис на
// себя (как при недоступности ведущего), и комната остаётся в его каталоге.
func (a *App) SkipRoomWait() error {
	a.mu.Lock()
	if a.rooms.config.Current == "" || a.rooms.config.Current == localRoomID {
		a.mu.Unlock()
		return nil
	}
	a.lobbySkipWait = true
	offline := time.Since(a.rooms.seen) >= time.Duration(config.Interface.RoomOnlineTimeoutMs)*time.Millisecond
	following := a.lobbyEnabled && a.appearance.RoomServiceURL == "" && a.lobbyHost != ""
	a.mu.Unlock()
	a.p2pLog("лобби: игрок прервал ожидание сервиса комнат")
	if offline && following {
		a.applyLobbyService("", "")
	}
	go a.roomTick()
	return nil
}

// EnableLobby включает публичное лобби: игроки ничего не вводят. Устройство читает общую тему ntfy,
// выбирает ведущего (самая старая установка среди живых) и пользуется его сервисом комнат.
// Если самая старая установка — это устройство, оно само объявляет свой сервис. Вызывается только
// обычным приложением (не проверкой связи и не тестами).
func (a *App) EnableLobby() {
	a.lobbyOnce.Do(func() {
		if os.Getenv("PREFERANS_ROOMS_URL") != "" {
			return // служебный HTTP-сервер для разработки: лобби не нужно
		}
		a.mu.Lock()
		a.lobbyEnabled, a.lobbyGatherSince, a.lobbyStart = true, time.Now(), time.Now()
		a.mu.Unlock()
		go a.lobbyLoop()
	})
}

func (a *App) lobbyLoop() {
	node, err := a.p2pNode()
	if err != nil {
		return
	}
	since := a.lobbySince
	if since == 0 {
		since = node.Since()
	}
	if a.lobbyMobile {
		node.SetMobile(true)
	}
	delay := 8 * time.Second
	if lobbyPeriod < delay {
		delay = lobbyPeriod
	}
	// Первые две минуты опрашиваем лобби чаще: relay на новом узле резервируется не сразу, и ведущий объявляется позже,
	// чем пройдёт первый период. Объявления при этом публикуются не чаще периода лобби.
	fast := 10 * time.Second
	if lobbyPeriod < fast {
		fast = lobbyPeriod
	}
	for ticks := 0; ; ticks++ {
		select {
		case <-a.closed:
			return
		case <-time.After(delay):
		case <-a.lobbyKick:
		}
		a.lobbyTick(node, since)
		delay = lobbyPeriod
		if ticks < 12 {
			delay = fast
		}
		// После обрыва связи (сеть вернулась, но relay ещё не зарезервирован или ведущий не отвечает) не ждём целый период:
		// повторяем опрос каждые 10 секунд, но не дольше двух минут подряд.
		if a.lobbyRetry > 0 && a.lobbyRetry <= 12 {
			delay = min(delay, fast)
		}
	}
}

func (a *App) lobbyTick(node *p2p.Node, since int64) {
	a.mu.Lock()
	manual := a.appearance.RoomServiceURL != ""
	hosting := a.lobbyHost == ""
	a.mu.Unlock()
	if manual {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	anns, err := p2p.ReadLobby(ctx, lobbyReadWindow())
	if err != nil {
		a.lobbyLog(fmt.Sprintf("лобби: объявления не прочитаны: %v", err))
		anns = nil
	}
	self := p2p.Announcement{ID: node.ID(), Since: since, At: time.Now().Unix()}
	cands := []p2p.Announcement{self}
	fresh := false
	for _, an := range anns {
		if an.ID != self.ID {
			cands = append(cands, an)
			fresh = fresh || time.Now().Unix()-an.At < int64(lobbyAlive()/time.Second)
		}
	}
	// Лобби недоступно (ntfy не отвечает): пробуем ведущего, которого запомнили при прошлой удачной встрече.
	remembered := false
	if saved := a.loadSavedHost(); saved != nil && saved.ID != self.ID && err != nil && !fresh && time.Since(a.lobbySavedTry) > min(20*time.Second, 4*lobbyPeriod) {
		a.lobbySavedTry = time.Now()
		an := p2p.Announcement{V: 1, ID: saved.ID, Addrs: saved.Addrs, Since: saved.Since, At: time.Now().Unix()}
		if a.syncRooms(ctx, node, an.Invite()) {
			cands = append(cands, an)
			remembered = true
			a.lobbyLog(fmt.Sprintf("лобби: ведущий %s найден по сохранённому адресу", shortID(an.ID)))
		}
	}
	if err != nil && !remembered {
		if hosting {
			a.lobbyAnnounce(ctx, node, since) // лобби недоступно для чтения: ведущий всё равно объявляется
		}
		return // иначе остаёмся на прежнем сервисе
	}
	self.Mobile = node.IsMobile()
	cands[0] = self
	// Кандидаты по приоритету: сначала настольные устройства, затем самая старая установка. Если лучший ведущий
	// недоступен именно этому устройству, берём следующего: иначе устройство осталось бы с мёртвым сервисом комнат
	// (так, в партии 14.11 телефон-ведущий уснул, а игроки разошлись по разным сервисам).
	ranked := p2p.RankHosts(cands, time.Now(), lobbyAlive())
	if len(ranked) == 0 {
		ranked = []p2p.Announcement{self}
	}
	// skipped — лучший кандидат жив (его объявление свежее), но этому устройству недоступен: он может быть доступен
	// другим игрокам, и тогда они выберут не того ведущего. Пока так, новые комнаты не создаются (см. lobbyUnsettled).
	skipped := false
	for _, winner := range ranked {
		if winner.ID == self.ID {
			a.lobbyLog(fmt.Sprintf("лобби: ведущий комнат — это устройство (живых кандидатов: %d)", len(cands)))
			// Прежний ведущий мог создавать комнаты без нас: перед объявлением вливаем их каталог в свой.
			unreachable := skipped
			for _, an := range anns {
				if an.ID != self.ID && time.Now().Unix()-an.At < int64(lobbyBlocking()/time.Second)+1 {
					if !a.syncRooms(ctx, node, an.Invite()) {
						unreachable = true
					}
				}
			}
			a.setLobbyUnsettled(unreachable)
			a.setLobbyDiverged(skipped)
			a.setLobbyReady()
			a.applyLobbyService("", "")
			a.lobbyAnnounce(ctx, node, since)
			a.syncOthers(node, ranked, self.ID)
			return
		}
		a.lobbyLog(fmt.Sprintf("лобби: ведущий комнат — %s (живых кандидатов: %d)", shortID(winner.ID), len(cands)))
		synced := a.syncRooms(ctx, node, winner.Invite())
		if !synced {
			fails := a.lobbyFail(winner.ID)
			skipped = true
			a.mu.Lock()
			current := a.lobbyHost == winner.ID
			inRoom := a.rooms.config.Current != "" && a.rooms.config.Current != localRoomID
			a.mu.Unlock()
			// Нынешнего ведущего не бросаем из-за одного сбоя, а новый кандидат, до которого не дотянуться, не получает сервис вовсе.
			if current && fails < lobbyFailLimit || inRoom && !current && fails < lobbyFailLimit {
				a.lobbyRetry++
				return
			}
			a.lobbyLog(fmt.Sprintf("лобби: ведущий %s недоступен этому устройству (неудач подряд: %d), берём следующего по приоритету", shortID(winner.ID), fails))
			continue
		}
		a.lobbyOK(winner.ID)
		a.setLobbyUnsettled(skipped)
		a.setLobbyDiverged(skipped)
		a.setLobbyReady()
		a.saveHost(winner)
		if code, err := p2p.Encode(p2p.Code{Kind: p2p.KindService, Invite: winner.Invite()}); err == nil {
			a.applyLobbyService(code, winner.ID)
		}
		a.lobbyAnnounce(ctx, node, since) // не только ведущий: все живые устройства видны друг другу, выбор ведущего един
		a.syncOthers(node, ranked, winner.ID)
		return
	}
}

// setLobbyDiverged запоминает, что устройство сидит не на сервисе лучшего живого ведущего: тот ему недоступен, а другим
// игрокам, возможно, доступен. Пока так, устройство не входит в комнаты и не опрашивает комнату: иначе оно оказалось бы
// участником комнаты на сервисе, которого другие не видят (партия 22.11: две «Север» с разными участниками).
func (a *App) setLobbyDiverged(v bool) {
	a.mu.Lock()
	changed := a.lobbyDiverged != v
	a.lobbyDiverged = v
	a.mu.Unlock()
	if changed && v {
		a.lobbyLog("лобби: лучший ведущий недоступен этому устройству, вход в комнаты приостановлен до встречи с ним (или «Не ждать»)")
	}
}

// roomsHeldLocked: вход в комнаты и опрос комнаты приостановлены (см. setLobbyDiverged). Игрок за столом не затрагивается.
func (a *App) roomsHeldLocked() bool {
	return a.lobbyEnabled && a.appearance.RoomServiceURL == "" && a.lobbyDiverged && !a.lobbySkipWait && a.saved.Table == "" && a.rooms.config.Table == ""
}

// setLobbyUnsettled запоминает, что часть живых игроков этому устройству недоступна. Пока это так, создание комнаты
// отклоняется: на разошедшихся сервисах появились бы две комнаты с одним названием. Повторный опрос — через 10 секунд.
func (a *App) setLobbyUnsettled(v bool) {
	a.mu.Lock()
	changed := a.lobbyUnsettled != v
	a.lobbyUnsettled = v
	if changed {
		if v {
			a.lobbyUnsettledSince = time.Now()
		} else {
			a.lobbySkipWait = false // все живые игроки снова доступны: обычный порядок
		}
	}
	a.mu.Unlock()
	if v {
		a.lobbyRetry++
	}
	if changed {
		if v {
			a.lobbyLog("лобби: часть живых игроков недоступна, создание комнат приостановлено")
		} else {
			a.lobbyLog("лобби: все живые игроки доступны, создание комнат разрешено")
		}
	}
}

// lobbyFailLimit — сколько опросов подряд ведущий может не отвечать, прежде чем нынешнего ведущего заменит следующий кандидат.
const lobbyFailLimit = 2

func (a *App) lobbyFail(id string) int {
	if a.lobbyFails == nil {
		a.lobbyFails = map[string]int{}
	}
	a.lobbyFails[id]++
	return a.lobbyFails[id]
}

func (a *App) lobbyOK(id string) {
	delete(a.lobbyFails, id)
	a.lobbyRetry = 0
}

// syncOthers вливает каталог комнат остальных живых кандидатов (не только выбранного ведущего): тогда комната, созданная
// на любом сервисе, попадает во все каталоги, и игроки находят её, даже если на время оказались на разных сервисах.
// Выполняется в фоне и не чаще одного обмена за раз: недоступный кандидат не задерживает опрос лобби.
func (a *App) syncOthers(node *p2p.Node, ranked []p2p.Announcement, skip string) {
	var targets []p2p.Announcement
	for _, an := range ranked {
		if an.ID != node.ID() && an.ID != skip && len(targets) < 2 {
			targets = append(targets, an)
		}
	}
	if len(targets) == 0 || !a.lobbySyncing.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer a.lobbySyncing.Store(false)
		for _, an := range targets {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			a.syncRooms(ctx, node, an.Invite())
			cancel()
		}
	}()
}

// lobbyLog пишет в журнал состояние лобби только при его изменении.
func (a *App) lobbyLog(note string) {
	a.mu.Lock()
	changed := a.lobbyNote != note
	a.lobbyNote = note
	a.mu.Unlock()
	if changed {
		a.p2pLog(note)
	}
}

// lobbyAnnounce публикует объявление не чаще периода лобби. Пока у узла нет адресов relay, повторяет на следующем опросе.
func (a *App) lobbyAnnounce(ctx context.Context, node *p2p.Node, since int64) {
	if time.Now().Before(a.lobbyBlockedTill) {
		return // ntfy ответил 429: не усугубляем, повторим после паузы
	}
	addrs := strings.Join(node.AnnounceAddrs(), ",")
	// Адреса relay сменились (после потери сети резервирование новое): ведущий объявляется сразу, иначе остальные
	// ещё целый период ищут его по устаревшим адресам.
	if addrs == a.lobbyAddrs && time.Since(a.lobbyAnnounced) < lobbyAnnounceEvery()-time.Second {
		return
	}
	if err := node.Announce(ctx, since); err != nil {
		if strings.Contains(err.Error(), "HTTP 429") {
			a.lobbyBlockedTill = time.Now().Add(lobbyBlockPause)
			a.lobbyLog("лобби: ntfy ограничил число сообщений (HTTP 429), объявления приостановлены на 5 минут")
			return
		}
		a.lobbyLog(fmt.Sprintf("лобби: объявление не отправлено: %v", err))
		a.lobbyRetry++ // повторяем опрос вне расписания, пока не появятся адреса relay
		return
	}
	a.lobbyAddrs = addrs
	a.lobbyRetry = 0
	first := a.lobbyAnnounced.IsZero()
	a.lobbyAnnounced = time.Now()
	if first {
		a.lobbyLog("лобби: объявление отправлено")
	}
}

// logEvery пишет сообщение в журнал сети не чаще раза в d для одного ключа (повторяющиеся неудачи не засоряют журнал).
func (a *App) logEvery(key string, d time.Duration, msg string) {
	a.mu.Lock()
	if a.logOnceAt == nil {
		a.logOnceAt = map[string]time.Time{}
	}
	due := time.Since(a.logOnceAt[key]) >= d
	if due {
		a.logOnceAt[key] = time.Now()
	}
	a.mu.Unlock()
	if due {
		a.p2pLog(msg)
	}
}

// syncRooms копирует долговечную часть каталога комнат (названия, надгробия, партии) с сервиса другого игрока
// в собственный сервис этого устройства по прямому соединению libp2p. Если это устройство позже станет ведущим,
// каталог у него уже есть. Ошибки не критичны: повтор через период лобби.
func (a *App) syncRooms(ctx context.Context, node *p2p.Node, inv p2p.Invite) bool {
	srv, err := a.roomService()
	if err != nil {
		return false
	}
	// Двусторонний обмен: отправляем свой снимок и получаем итоговый. Так комнаты, созданные на этом устройстве,
	// появляются у ведущего, и игрок остаётся в своей комнате при переходе на его сервис.
	body, _ := json.Marshal(rooms.Request{Op: "sync", Snapshot: srv.Snapshot()})
	raw, err := node.Request(ctx, inv, body)
	if err != nil {
		a.logEvery("sync-fail-"+inv.ID, 5*time.Minute, fmt.Sprintf("каталог комнат: нет связи с %s: %v", shortID(inv.ID), err))
		return false
	}
	var out rooms.Response
	if json.Unmarshal(raw, &out) != nil || out.Error != "" {
		a.p2pLog(fmt.Sprintf("каталог комнат: %s не принял обмен: %s", shortID(inv.ID), out.Error))
		return false
	}
	switch n, err := srv.Merge(out.Snapshot); {
	case err != nil:
		a.p2pLog(fmt.Sprintf("каталог комнат: слияние не удалось: %v", err))
		return false
	case n > 0:
		a.p2pLog(fmt.Sprintf("каталог комнат: обновлено комнат %d (с %s)", n, shortID(inv.ID)))
	}
	return true
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[len(id)-8:]
	}
	return id
}

// applyLobbyService переключает сервис комнат. Смена ведущего не происходит, пока игрок за столом;
// обновление адресов того же ведущего допустимо всегда.
func (a *App) applyLobbyService(url, hostID string) {
	a.roomControl.Lock()
	defer a.roomControl.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.rooms.url == url && a.lobbyHost == hostID {
		return
	}
	same := a.lobbyHost == hostID
	// Игрок в комнате переходит вместе с комнатой (каталоги уже обменяны); идущий стол не прерываем.
	// Завершённый стол (результат на экране) и стол без единого соединения — «хвосты» прошлой партии: они не должны
	// держать игрока на сервисе, которого другие игроки не видят (так 17.11 игрок оказался в комнате один).
	// Стол без соединений отпускаем только в первые две минуты после запуска (стол прошлого вечера, восстановленный при старте).
	busy := a.saved.Table != "" && !savedFinished(a.saved) && (len(a.peers) > 0 || time.Since(a.lobbyStart) >= 2*time.Minute)
	if busy && !same {
		return
	}
	inRoom := a.rooms.config.Current != "" && a.rooms.config.Current != localRoomID
	a.lobbyHost = hostID
	a.rooms.url = url
	if !same {
		go a.p2pLog(fmt.Sprintf("лобби: сервис комнат переключён (ведущий %s; игрок в комнате: %t)", map[bool]string{true: "это устройство", false: shortID(hostID)}[hostID == ""], inRoom))
		a.rooms.generation++
		a.rooms.offer = nil
		a.rooms.err = ""
		a.rooms.failures, a.rooms.retryAt = 0, time.Time{}
		go a.roomTick() // сразу заново войти в комнату на новом сервисе, не ждать следующего опроса
	}
}

// NetworkChanged сообщает, что сеть могла измениться или приложение вернулось из фона (на Android — экран разблокирован,
// смена Wi-Fi/LTE). Узел проверяет соединения и сбрасывает штрафы повторных попыток, лобби и опрос комнаты
// выполняются сразу, а не по расписанию. Возвращается немедленно.
func (a *App) NetworkChanged() {
	a.mu.Lock()
	node := a.node
	a.rooms.failures, a.rooms.retryAt = 0, time.Time{} // сеть изменилась: опрос комнаты без паузы
	if a.lobbyEnabled {
		a.lobbyReady, a.lobbyGatherSince = false, time.Now() // после смены сети сервис комнат определяется заново
	}
	a.mu.Unlock()
	if node == nil {
		return
	}
	go func() {
		node.NetworkChanged()
		select {
		case a.lobbyKick <- struct{}{}:
		default:
		}
		a.roomTick()
	}()
}

// savedHost — ведущий, с которым удалось обменяться каталогами: его запоминаем на диске, чтобы найти и без лобби
// (ntfy недоступен) сразу после запуска. Адреса relay могут устареть — тогда попытка просто не удастся.
type savedHost struct {
	ID    string   `json:"id"`
	Addrs []string `json:"addrs"`
	Since int64    `json:"since"`
}

func (a *App) loadSavedHost() *savedHost {
	var h savedHost
	if a.config.Load("lobby-host", &h) != nil || h.ID == "" || len(h.Addrs) == 0 || len(h.Addrs) > 6 || h.Since <= 0 {
		return nil
	}
	return &h
}

func (a *App) saveHost(an p2p.Announcement) {
	if an.ID == "" || len(an.Addrs) == 0 {
		return
	}
	if cur := a.loadSavedHost(); cur != nil && cur.ID == an.ID && cur.Since == an.Since && strings.Join(cur.Addrs, ",") == strings.Join(an.Addrs, ",") {
		return
	}
	_ = a.config.Save("lobby-host", savedHost{ID: an.ID, Addrs: an.Addrs, Since: an.Since})
}