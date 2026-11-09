package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"preferans/internal/p2p"
	"preferans/internal/rooms"
)

// lobbyPeriod — период опроса и объявления в публичном лобби.
var lobbyPeriod = 60 * time.Second

// EnableLobby включает публичное лобби: игроки ничего не вводят. Устройство читает общую тему ntfy,
// выбирает ведущего (самая старая установка среди живых) и пользуется его сервисом комнат.
// Если самая старая установка — это устройство, оно само объявляет свой сервис. Вызывается только
// обычным приложением (не проверкой связи и не тестами).
func (a *App) EnableLobby() {
	a.lobbyOnce.Do(func() {
		if os.Getenv("PREFERANS_ROOMS_URL") != "" {
			return // служебный HTTP-сервер для разработки: лобби не нужно
		}
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
	anns, err := p2p.ReadLobby(ctx, 4*lobbyPeriod)
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
			fresh = fresh || time.Now().Unix()-an.At < int64((3*lobbyPeriod)/time.Second)
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
	winner, ok := p2p.ChooseHost(cands, time.Now(), 3*lobbyPeriod)
	if !ok {
		winner = self
	}
	if winner.ID == self.ID {
		a.lobbyLog(fmt.Sprintf("лобби: ведущий комнат — это устройство (живых кандидатов: %d)", len(cands)))
		// Прежний ведущий мог создавать комнаты без нас: перед объявлением вливаем их каталог в свой.
		for _, an := range anns {
			if an.ID != self.ID && time.Now().Unix()-an.At < int64((3*lobbyPeriod)/time.Second)+1 {
				a.syncRooms(ctx, node, an.Invite())
			}
		}
		a.applyLobbyService("", "")
		a.lobbyAnnounce(ctx, node, since)
		return
	}
	a.lobbyLog(fmt.Sprintf("лобби: ведущий комнат — %s (живых кандидатов: %d)", shortID(winner.ID), len(cands)))
	synced := a.syncRooms(ctx, node, winner.Invite())
	a.mu.Lock()
	inRoom := a.rooms.config.Current != "" && a.rooms.config.Current != localRoomID
	a.mu.Unlock()
	if inRoom && !synced {
		return // без обмена каталогами переход оборвал бы комнату: повторим через период
	}
	if synced {
		a.saveHost(winner)
	}
	code, err := p2p.Encode(p2p.Code{Kind: p2p.KindService, Invite: winner.Invite()})
	if err == nil {
		a.applyLobbyService(code, winner.ID)
	}
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
	if time.Since(a.lobbyAnnounced) < lobbyPeriod-time.Second {
		return
	}
	if err := node.Announce(ctx, since); err != nil {
		a.lobbyLog(fmt.Sprintf("лобби: объявление не отправлено: %v", err))
		return
	}
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
	busy := a.saved.Table != ""
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
	}
}

// NetworkChanged сообщает, что сеть могла измениться или приложение вернулось из фона (на Android — экран разблокирован,
// смена Wi-Fi/LTE). Узел проверяет соединения и сбрасывает штрафы повторных попыток, лобби и опрос комнаты
// выполняются сразу, а не по расписанию. Возвращается немедленно.
func (a *App) NetworkChanged() {
	a.mu.Lock()
	node := a.node
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