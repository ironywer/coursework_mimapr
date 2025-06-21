package p2p

import (
	"context"
	"fmt"
	"log"

	host "github.com/libp2p/go-libp2p/core/host"
	peerstore "github.com/libp2p/go-libp2p/core/peer"
)

// ReportResult отправляет на сервер информацию об успешной или неуспешной
// обработке изображения. ok=true означает успех.
func ReportResult(h host.Host, server peerstore.AddrInfo, processor peerstore.ID, ok bool) {
	stream, err := h.NewStream(context.Background(), server.ID, "/report-result/1.0.0")
	if err != nil {
		log.Println("❌ Ошибка подключения для отправки отчета:", err)
		return
	}
	defer stream.Close()

	status := "FAIL"
	if ok {
		status = "OK"
	}
	msg := fmt.Sprintf("%s|%s", processor.String(), status)
	if _, err := stream.Write([]byte(msg)); err != nil {
		log.Println("❌ Ошибка отправки отчета:", err)
	}
}
