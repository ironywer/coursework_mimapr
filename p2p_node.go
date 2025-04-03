package main

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	host "github.com/libp2p/go-libp2p/core/host"
	network "github.com/libp2p/go-libp2p/core/network"
	peerstore "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

func main() {
	// Создаём P2P-узел с адресом
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("📡 Узел запущен с адресами:")
	for _, addr := range h.Addrs() {
		fmt.Printf(" - %s/p2p/%s\n", addr, h.ID().String())
	}

	// Добавляем собственные адреса в Peerstore
	selfInfo := peerstore.AddrInfo{ID: h.ID(), Addrs: h.Addrs()}
	h.Peerstore().AddAddrs(selfInfo.ID, selfInfo.Addrs, time.Hour)

	// Читаем bootstrap-адрес
	bootstrapAddrStr, err := os.ReadFile("bootstrap.txt")
	if err != nil {
		log.Fatal("❌ Ошибка чтения bootstrap.txt:", err)
	}
	bootstrapAddr := strings.TrimSpace(string(bootstrapAddrStr))
	maddr, _ := ma.NewMultiaddr(bootstrapAddr)
	bootstrapInfo, _ := peerstore.AddrInfoFromP2pAddr(maddr)

	// Подключаемся к серверу
	err = h.Connect(context.Background(), *bootstrapInfo)
	if err != nil {
		log.Fatal("❌ Ошибка подключения к серверу:", err)
	}
	fmt.Println("✅ Подключен к серверу:", bootstrapInfo.ID)

	// Устанавливаем обработчик получения изображений
	h.SetStreamHandler("/receive-image/1.0.0", receiveImage)

	// Запрашиваем путь к папке
	fmt.Print("\n📂 Введите путь к папке с изображениями: ")
	reader := bufio.NewReader(os.Stdin)
	dirPath, _ := reader.ReadString('\n')
	dirPath = strings.TrimSpace(dirPath)

	files, err := os.ReadDir(dirPath)
	if err != nil {
		log.Fatal("❌ Ошибка чтения папки:", err)
	}

	for _, file := range files {
		if file.IsDir() || !isImageFile(file) {
			continue
		}

		imagePath := filepath.Join(dirPath, file.Name())

		// Запрашиваем получателя у сервера
		receiverID, receiverAddrs := requestPeer(h, *bootstrapInfo)
		if receiverID == "" {
			fmt.Println("⚠️ Пропускаем файл, нет доступных получателей:", file.Name())
			continue
		}

		// Добавляем получателя в Peerstore
		receiverInfo := peerstore.AddrInfo{ID: receiverID, Addrs: receiverAddrs}
		h.Peerstore().AddAddrs(receiverID, receiverAddrs, time.Hour)

		// Отправляем файл
		fmt.Printf("📤 Отправляем %s ➜ %s...\n", file.Name(), receiverID)
		sendImage(h, receiverInfo, imagePath)
	}

	fmt.Println("✅ Все изображения обработаны.")
}

// Проверка на расширение файла
func isImageFile(file fs.DirEntry) bool {
	name := strings.ToLower(file.Name())
	return strings.HasSuffix(name, ".jpg") || strings.HasSuffix(name, ".jpeg") || strings.HasSuffix(name, ".png")
}

// Запрос назначения у сервера
func requestPeer(h host.Host, server peerstore.AddrInfo) (peerstore.ID, []ma.Multiaddr) {
	stream, err := h.NewStream(context.Background(), server.ID, "/request-peer/1.0.0")
	if err != nil {
		log.Println("❌ Ошибка запроса назначения:", err)
		return "", nil
	}
	defer stream.Close()

	buf := make([]byte, 1024)
	n, err := stream.Read(buf)
	if err != nil {
		log.Println("❌ Ошибка чтения ответа:", err)
		return "", nil
	}

	resp := string(buf[:n])
	if resp == "NO_PEER" {
		return "", nil
	}

	parts := strings.Split(resp, "|")
	if len(parts) < 2 {
		return "", nil
	}

	peerID, _ := peerstore.Decode(parts[0])
	var addrs []ma.Multiaddr
	for _, s := range strings.Split(parts[1], ",") {
		a, err := ma.NewMultiaddr(s)
		if err == nil {
			addrs = append(addrs, a)
		}
	}
	return peerID, addrs
}

// Отправка файла другому пиру
func sendImage(h host.Host, receiver peerstore.AddrInfo, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Println("❌ Ошибка чтения файла:", err)
		return
	}

	stream, err := h.NewStream(context.Background(), receiver.ID, "/receive-image/1.0.0")
	if err != nil {
		log.Println("❌ Ошибка соединения:", err)
		return
	}
	defer stream.Close()

	_, err = stream.Write(data)
	if err != nil {
		log.Println("❌ Ошибка отправки:", err)
		return
	}

	fmt.Println("✅ Файл успешно отправлен:", filepath.Base(path))
}

// Получение изображения
func receiveImage(s network.Stream) {
	fmt.Println("📥 Получение изображения от:", s.Conn().RemotePeer())

	// Сохраняем файл
	fileName := fmt.Sprintf("received_%s.jpg", s.Conn().RemotePeer().String())
	file, err := os.Create(fileName)
	if err != nil {
		log.Println("❌ Ошибка создания файла:", err)
		return
	}
	defer file.Close()

	buf := make([]byte, 4096)
	for {
		n, err := s.Read(buf)
		if n > 0 {
			file.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}

	fmt.Println("✅ Файл получен и сохранён как:", fileName)
}
