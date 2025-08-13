package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	p2p "coursework_mimapr/internal/p2p"

	libp2p "github.com/libp2p/go-libp2p"

	host "github.com/libp2p/go-libp2p/core/host"
	network "github.com/libp2p/go-libp2p/core/network"
	peer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

const keyFile = "server_key.pem"

var (
	peers     = make(map[peer.ID]peer.AddrInfo)
	peerList  []peer.ID
	nextIndex int
	lock      sync.Mutex
)

func main() {
	// if err := db.Init("tokens.db"); err != nil {
	// 	log.Fatal("❌ Не удалось инициализировать БД:", err)
	// }
	// log.Println("✅ БД подключена и таблица users готова")
	privKey, err := p2p.LoadOrCreateKey(keyFile)
	if err != nil {
		log.Fatal(err)
	}

	h, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/9000"),
		libp2p.Identity(privKey),
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("🚀 Сервер запущен! ID:", h.ID())

	// ——————————————————————————————————————
	// Записываем свой адрес в bootstrap.txt
	// addrs := h.Addrs()
	// if len(addrs) == 0 {
	// 	log.Fatal("❌ Не удалось получить адреса хоста для записи в bootstrap.txt")
	//}
	// Вместо ip4/0.0.0.0 используем DNS-имя сервиса или локальный адрес
	hostName := os.Getenv("BOOTSTRAP_HOST")
	if hostName == "" {
		hostName = "localhost"
	}
	fmt.Println("🛰 Используется bootstrap host:", hostName)
	bootstrapLine := fmt.Sprintf("/dns4/%s/tcp/9000/p2p/%s\n",
		hostName, h.ID().String(),
	)
	if err := os.WriteFile("bootstrap.txt", []byte(bootstrapLine), 0644); err != nil {
		log.Fatalf("❌ Не удалось записать bootstrap.txt: %v", err)
	}
	fmt.Println("✅ Записан bootstrap multiaddr:", bootstrapLine)

	h.SetStreamHandler("/request-peer/1.0.0", handlePeerRequest)

	h.Network().Notify(&network.NotifyBundle{
		ConnectedF:    func(n network.Network, c network.Conn) { onPeerConnected(n, c, h) },
		DisconnectedF: onPeerDisconnected,
	})

	select {}
}

// Загружаем или создаём приватный ключ

// Обработчик подключения
func onPeerConnected(net network.Network, conn network.Conn, h host.Host) {
	lock.Lock()
	defer lock.Unlock()

	peerID := conn.RemotePeer()

	// Ждём до 5 секунд, пока появятся адреса
	var addrs []ma.Multiaddr
	for i := 0; i < 5; i++ {
		addrs = h.Peerstore().Addrs(peerID)
		if len(addrs) > 0 {
			break
		}
		time.Sleep(time.Second)
	}

	// Если адресов все ещё нет, используем адрес из соединения
	if len(addrs) == 0 {
		if maddr := conn.RemoteMultiaddr(); maddr != nil {
			addrs = []ma.Multiaddr{maddr}
		}
	}

	if len(addrs) == 0 {
		fmt.Println("⚠️ У пира нет известных адресов, он не будет добавлен:", peerID)
		return
	}

	peers[peerID] = peer.AddrInfo{ID: peerID, Addrs: addrs}
	peerList = append(peerList, peerID)
	fmt.Println("🔗 Новый пир подключен:", peerID)
}

// Обработчик отключения
func onPeerDisconnected(net network.Network, conn network.Conn) {
	lock.Lock()
	defer lock.Unlock()

	peerID := conn.RemotePeer()
	delete(peers, peerID)

	// Удаляем из списка peerList
	for i, id := range peerList {
		if id == peerID {
			peerList = append(peerList[:i], peerList[i+1:]...)
			break
		}
	}

	fmt.Println("❌ Пир отключен:", peerID)
}

// Назначение пира по кругу
func handlePeerRequest(s network.Stream) {
	lock.Lock()
	defer lock.Unlock()

	sender := s.Conn().RemotePeer()
	var receiverID peer.ID

	if len(peerList) <= 1 {
		fmt.Println("❌ Недостаточно пиров для распределения.")
		s.Write([]byte("NO_PEER"))
		s.Close()
		return
	}

	// Находим следующего пира ≠ отправителю
	tries := 0
	for tries < len(peerList) {
		receiverID = peerList[nextIndex%len(peerList)]
		nextIndex++
		if receiverID != sender {
			break
		}
		tries++
	}

	receiverInfo, ok := peers[receiverID]
	if !ok || len(receiverInfo.Addrs) == 0 {
		fmt.Println("⚠️ Назначенный пир невалидный:", receiverID)
		s.Write([]byte("NO_PEER"))
		s.Close()
		return
	}

	var addrList []string
	for _, addr := range receiverInfo.Addrs {
		addrList = append(addrList, addr.String())
	}
	response := fmt.Sprintf("%s|%s", receiverID.String(), strings.Join(addrList, ","))

	s.Write([]byte(response))
	s.Close()

	fmt.Printf("📤 Назначен получатель для %s ➜ %s\n", sender, receiverID)
}
