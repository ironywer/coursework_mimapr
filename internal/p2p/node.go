package p2p

import (
	"context"
	"log"
	"strings"

	peerstore "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"

	host "github.com/libp2p/go-libp2p/core/host"
)

// RequestPeer отправляет запрос серверу на получение адреса обработчика.
// Третий возвращаемый аргумент показывает, что у инициатора закончились токены.
func RequestPeer(h host.Host, server peerstore.AddrInfo) (peerstore.ID, []ma.Multiaddr, bool) {
	stream, err := h.NewStream(context.Background(), server.ID, "/request-peer/1.0.0")
	if err != nil {
		log.Println("❌ Ошибка запроса назначения:", err)
		return "", nil, false
	}
	defer stream.Close()

	buf := make([]byte, 1024)
	n, err := stream.Read(buf)
	if err != nil {
		log.Println("❌ Ошибка чтения ответа от сервера:", err)
		return "", nil, false
	}
	resp := string(buf[:n])
	if resp == "NO_PEER" {
		return "", nil, false
	}
	if resp == "NO_TOKENS" {
		return "", nil, true
	}
	parts := strings.Split(resp, "|")
	if len(parts) < 2 {
		return "", nil, false
	}
	peerID, _ := peerstore.Decode(parts[0])
	var addrs []ma.Multiaddr
	for _, s := range strings.Split(parts[1], ",") {
		a, err := ma.NewMultiaddr(s)
		if err == nil {
			addrs = append(addrs, a)
		}
	}
	return peerID, addrs, false
}
