package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	crypto "github.com/libp2p/go-libp2p/core/crypto"
	peer "github.com/libp2p/go-libp2p/core/peer"
	network "github.com/libp2p/go-libp2p/core/network"
	host "github.com/libp2p/go-libp2p/core/host"
	ma "github.com/multiformats/go-multiaddr"
)

const keyFile = "bootstrap_key.pem"

var (
	peers     = make(map[peer.ID]peer.AddrInfo)
	peerList  []peer.ID
	nextIndex int
	lock      sync.Mutex
)

func main() {
	privKey, err := loadOrCreateKey()
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

	h.SetStreamHandler("/request-peer/1.0.0", handlePeerRequest)

	h.Network().Notify(&network.NotifyBundle{
		ConnectedF:    func(n network.Network, c network.Conn) { onPeerConnected(n, c, h) },
		DisconnectedF: onPeerDisconnected,
	})

	select {}
}

// Загружаем или создаём приватный ключ
func loadOrCreateKey() (crypto.PrivKey, error) {
	if _, err := os.Stat(keyFile); err == nil {
		data, err := os.ReadFile(keyFile)
		if err != nil {
			return nil, err
		}
		return crypto.UnmarshalPrivateKey(data)
	}

	privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, -1)
	if err != nil {
		return nil, err
	}
	data, err := crypto.MarshalPrivateKey(privKey)
	if err != nil {
		return nil, err
	}
	os.WriteFile(keyFile, data, 0600)
	return privKey, nil
}

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
