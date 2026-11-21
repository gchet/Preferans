package app

import (
	"fmt"
	"strings"
)

// netKindName переводит перечень транспортов, который передаёт оболочка Android («wifi+vpn»), в текст для журнала.
func netKindName(kind string) string {
	names := map[string]string{"wifi": "Wi-Fi", "cellular": "мобильная сеть", "ethernet": "Ethernet", "vpn": "VPN", "other": "другая сеть", "none": "нет сети"}
	var out []string
	for _, k := range strings.Split(kind, "+") {
		if n, ok := names[k]; ok {
			out = append(out, n)
		} else if k != "" {
			out = append(out, k)
		}
	}
	return strings.Join(out, " + ")
}

// SetNetworkKind запоминает тип основной сети устройства, о котором сообщила оболочка (Android: ConnectivityManager),
// и пишет в журнал только его смену. По журналу видно, что телефон ушёл с домашней Wi-Fi на мобильную сеть: после этого
// игрок оказывается не в локальной сети ведущего, и все соединения идут по публичным адресам.
func (a *App) SetNetworkKind(kind string, validated bool) {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return
	}
	if !validated && kind != "none" {
		kind += "?"
	}
	a.mu.Lock()
	prev := a.netKind
	changed := prev != kind
	a.netKind = kind
	a.mu.Unlock()
	if !changed {
		return
	}
	text := strings.TrimSuffix(kind, "?")
	note := netKindName(text)
	if strings.HasSuffix(kind, "?") {
		note += " (доступ в интернет пока не подтверждён)"
	}
	if prev != "" {
		old := netKindName(strings.TrimSuffix(prev, "?"))
		a.p2pLog(fmt.Sprintf("тип подключения устройства: %s (было: %s)", note, old))
		return
	}
	a.p2pLog(fmt.Sprintf("тип подключения устройства: %s", note))
}