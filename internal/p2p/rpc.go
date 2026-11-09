package p2p

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	RequestProtocol   = "/preferans/rooms/1.0.0"
	MaxRequest        = 4 * 1024 * 1024
	requestTimeout    = 20 * time.Second
	requestDialBudget = 10 * time.Second // соединение и открытие потока; попытка дольше — повторяется опросом комнаты каждую секунду
)

// HandleRequest включает на этом узле сервис «запрос-ответ» (сервис комнат).
// f получает тело запроса и peer ID отправителя и возвращает тело ответа.
func (n *Node) HandleRequest(f func(from string, req []byte) []byte) {
	n.h.SetStreamHandler(RequestProtocol, func(s network.Stream) {
		defer s.Close()
		_ = s.SetDeadline(time.Now().Add(requestTimeout))
		r := bufio.NewReader(s)
		l, err := binary.ReadUvarint(r)
		if err != nil || l == 0 || l > MaxRequest {
			_ = s.Reset()
			return
		}
		req := make([]byte, l)
		if _, err := io.ReadFull(r, req); err != nil {
			_ = s.Reset()
			return
		}
		resp := f(s.Conn().RemotePeer().String(), req)
		if len(resp) > MaxRequest {
			_ = s.Reset()
			return
		}
		var hdr [binary.MaxVarintLen64]byte
		k := binary.PutUvarint(hdr[:], uint64(len(resp)))
		_, _ = s.Write(append(hdr[:k:k], resp...))
	})
}

// Request отправляет запрос узлу с приглашением inv и ждёт ответ. Если адресов нет,
// они берутся из публичного объявления узла (Resolve).
func (n *Node) Request(ctx context.Context, inv Invite, body []byte) ([]byte, error) {
	if len(body) > MaxRequest {
		return nil, errors.New("запрос слишком большой")
	}
	id, err := peer.Decode(inv.ID)
	if err != nil {
		return nil, fmt.Errorf("неверный peer id: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, requestDialBudget)
	defer cancel()
	if n.h.Network().Connectedness(id) != network.Connected {
		if len(inv.Addrs) == 0 && !n.cfg.LoopbackOnly {
			if inv, err = n.Resolve(ctx, inv.ID); err != nil {
				return nil, err
			}
		}
		if err := n.connect(ctx, inv); err != nil {
			return nil, err
		}
	}
	s, err := n.h.NewStream(network.WithAllowLimitedConn(ctx, "preferans"), id, RequestProtocol)
	if err != nil && len(inv.Addrs) > 0 {
		// Соединение могло зависнуть (сеть пропала, пока приложение было в фоне): закрываем его и соединяемся заново.
		_ = n.h.Network().ClosePeer(id)
		if cerr := n.connect(ctx, inv); cerr == nil {
			s, err = n.h.NewStream(network.WithAllowLimitedConn(ctx, "preferans"), id, RequestProtocol)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("поток: %w", err)
	}
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(requestTimeout))
	var hdr [binary.MaxVarintLen64]byte
	k := binary.PutUvarint(hdr[:], uint64(len(body)))
	if _, err := s.Write(append(hdr[:k:k], body...)); err != nil {
		return nil, err
	}
	r := bufio.NewReader(s)
	l, err := binary.ReadUvarint(r)
	if err != nil || l > MaxRequest {
		return nil, errors.New("нет ответа от сервиса")
	}
	var out bytes.Buffer
	if _, err := io.CopyN(&out, r, int64(l)); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}