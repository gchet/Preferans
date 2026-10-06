package network

import "preferans/locales"
import "preferans/config"

import (
	"context"
	"crypto/tls"
	"errors"

	"net"
	"strconv"
	"strings"

	"github.com/pion/stun/v4"
	"github.com/pion/turn/v5"
)

var DefaultTURN = config.Network.TurnServer

// CheckTURN obtains and releases a real allocation; a STUN response is not enough.
func CheckTURN(ctx context.Context, entry string) (string, error) {
	p := strings.Split(entry, "|")
	if len(p) != 3 || p[1] == "" || p[2] == "" {
		return "", locales.Errorf("go.internal.network.turn_check.text001")
	}
	u, err := stun.ParseURI(p[0])
	if err != nil || (u.Scheme != stun.SchemeTypeTURN && u.Scheme != stun.SchemeTypeTURNS) {
		return "", locales.Errorf("go.internal.network.turn_check.text002")
	}
	var conn net.PacketConn
	host := net.JoinHostPort(u.Host, strconv.Itoa(u.Port))
	if u.Proto == stun.ProtoTypeTCP {
		var stream net.Conn
		if u.Scheme == stun.SchemeTypeTURNS {
			dialer := tls.Dialer{Config: &tls.Config{ServerName: u.Host, MinVersion: tls.VersionTLS12}}
			stream, err = dialer.DialContext(ctx, "tcp", host)
		} else {
			stream, err = (&net.Dialer{}).DialContext(ctx, "tcp", host)
		}
		if err == nil {
			conn = turn.NewSTUNConn(stream)
		}
	} else if u.Scheme == stun.SchemeTypeTURN {
		ips, e := net.DefaultResolver.LookupIPAddr(ctx, u.Host)
		if e != nil {
			return "", locales.Errorf("go.internal.network.turn_check.text003")
		}
		for _, ip := range ips {
			if ip.IP.To4() != nil {
				host = net.JoinHostPort(ip.IP.String(), strconv.Itoa(u.Port))
				break
			}
		}
		conn, err = net.ListenPacket("udp4", "0.0.0.0:0")
	} else {
		return "", locales.Errorf("go.internal.network.turn_check.text004")
	}
	if err != nil {
		return "", locales.Errorf("go.internal.network.turn_check.text005")
	}
	defer conn.Close()
	client, err := turn.NewClient(&turn.ClientConfig{TURNServerAddr: host, Conn: conn, Username: p[1], Password: p[2]})
	if err != nil {
		return "", locales.Errorf("go.internal.network.turn_check.text006")
	}
	defer client.Close()
	stop := context.AfterFunc(ctx, func() { client.Close(); _ = conn.Close() })
	defer stop()
	if err = client.Listen(); err != nil {
		return "", locales.Errorf("go.internal.network.turn_check.text007")
	}
	relay, err := client.Allocate()
	if err != nil {
		var auth *stun.TurnError
		if errors.As(err, &auth) && (auth.ErrorCodeAttr.Code == 401 || auth.ErrorCodeAttr.Code == 441) {
			return "", locales.Errorf("go.internal.network.turn_check.text008")
		}
		if ctx.Err() != nil {
			return "", locales.Errorf("go.internal.network.turn_check.text009")
		}
		if errors.As(err, &auth) {
			return "", locales.Errorf("go.internal.network.turn_check.text010", auth.ErrorCodeAttr.Code)
		}
		return "", locales.Errorf("go.internal.network.turn_check.text011")
	}
	defer relay.Close()
	return relay.LocalAddr().String(), nil
}
