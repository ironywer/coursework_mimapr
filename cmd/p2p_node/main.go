package main

import (
	"bufio"
	"context"
	p2p "coursework_mimapr/internal/p2p"
	"coursework_mimapr/internal/style"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	network "github.com/libp2p/go-libp2p/core/network"
	peerstore "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

var styleFile = "style.pt" // Файл, в котором будут признаки стиля
const (
	initiatorKeyFile = "initiator_key.pem"
	processorKeyFile = "processor_key.pem"
)

// sentStyle отслеживает, для каких пиров уже отправлены признаки стиля.
var sentStyle = make(map[peerstore.ID]bool)

func main() {
	// Определяем режим работы: "initiator" или "processor" (по умолчанию initiator)
	mode := "initiator"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	fmt.Println("Режим работы:", mode)

	// Выбираем файл ключа и загружаем/создаём его
	keyPath := initiatorKeyFile
	if mode == "processor" {
		keyPath = processorKeyFile
	}
	privKey, err := p2p.LoadOrCreateKey(keyPath)
	if err != nil {
		log.Fatal(err)
	}

	// Создаем P2P-узел с открытым портом и собственной идентичностью
	h, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"),
		libp2p.Identity(privKey),
	)
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

	// Подключаемся к bootstrap-серверу
	bootstrapAddrStr, err := os.ReadFile("bootstrap.txt")
	if err != nil {
		log.Fatal("❌ Ошибка чтения bootstrap.txt:", err)
	}
	bootstrapAddr := strings.TrimSpace(string(bootstrapAddrStr))
	maddr, err := ma.NewMultiaddr(bootstrapAddr)
	if err != nil {
		log.Fatal("❌ Ошибка парсинга bootstrap-адреса:", err)
	}
	bootstrapInfo, err := peerstore.AddrInfoFromP2pAddr(maddr)
	if err != nil {
		log.Fatal("❌ Ошибка преобразования в PeerInfo:", err)
	}
	err = h.Connect(context.Background(), *bootstrapInfo)
	if err != nil {
		log.Fatal("❌ Ошибка подключения к серверу:", err)
	}
	fmt.Println("✅ Подключен к серверу:", bootstrapInfo.ID)

	// Если режим processor, регистрируем обработчики для приема стиля и изображений
	if mode == "processor" {
		h.SetStreamHandler("/receive-style/1.0.0", p2p.HandleReceiveStyle)
		h.SetStreamHandler("/receive-image/1.0.0", p2p.MakeReceiveImageHandler(h))
		fmt.Println("🔧 Режим процессора: обработчики для /receive-style/1.0.0 и /receive-image/1.0.0 зарегистрированы.")
		// Режим процессора работает только для обработки входящих данных
		select {}
	} else {
		h.SetStreamHandler("/receive-image-result/1.0.0", func(s network.Stream) {
			defer s.Close()
			reader := bufio.NewReader(s)

			header, err := reader.ReadString('\n')
			if err != nil {
				log.Println("❌ Ошибка чтения результата:", err)
				return
			}
			header = strings.TrimSpace(header)

			if header == "ERROR" {
				msg, _ := reader.ReadString('\n')
				log.Println("❌ Процессор сообщил об ошибке:", strings.TrimSpace(msg))
				return
			}

			if header == "IMAGE" {
				timestamp := time.Now().UnixNano()
				dir := "processed_images"
				os.MkdirAll(dir, 0755)
				fileName := fmt.Sprintf("%s/styled_%d.jpg", dir, timestamp)
				file, err := os.Create(fileName)
				if err != nil {
					log.Println("❌ Ошибка создания файла результата:", err)
					return
				}
				defer file.Close()
				_, err = io.Copy(file, reader)
				if err != nil {
					log.Println("❌ Ошибка сохранения результата:", err)
					return
				}
				log.Println("✅ Обработанный файл получен:", fileName)
			}
		})

	}
	// Режим инициатора:
	// 1. Извлекаем стиль
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("\n🖌 Введите путь к изображению-стилю: ")
	styleImgPath, _ := reader.ReadString('\n')
	styleImgPath = strings.TrimSpace(styleImgPath)

	fmt.Println("⏳ Извлечение признаков стиля...")
	cmd := exec.Command(style.GetPythonCommand(), "style_transfer.py", "extract-style", styleImgPath, styleFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatal("❌ Ошибка извлечения стиля:", err)
	}
	fmt.Println("✅ Признаки стиля сохранены в", styleFile)

	// 2. Запрашиваем путь к папке с изображениями для стилизации
	fmt.Print("\n📂 Введите путь к папке с изображениями для стилизации: ")
	dirPath, _ := reader.ReadString('\n')
	dirPath = strings.TrimSpace(dirPath)
	files, err := os.ReadDir(dirPath)
	if err != nil {
		log.Fatal("❌ Ошибка чтения папки:", err)
	}

	// Для каждого изображения запрашиваем получателя, отправляем стиль (если еще не отправлен) и само изображение
	for _, file := range files {
		if file.IsDir() || !style.IsImageFile(file) {
			continue
		}
		imagePath := filepath.Join(dirPath, file.Name())

		receiverID, receiverAddrs := p2p.RequestPeer(h, *bootstrapInfo)
		if receiverID == "" {
			fmt.Println("⚠️ Нет доступных получателей для файла:", file.Name())
			continue
		}
		receiverInfo := peerstore.AddrInfo{
			ID:    receiverID,
			Addrs: receiverAddrs,
		}
		h.Peerstore().AddAddrs(receiverInfo.ID, receiverInfo.Addrs, time.Hour)

		err = h.Connect(context.Background(), receiverInfo)
		if err != nil {
			log.Println("❌ Ошибка подключения к получателю для отправки результата:", err)
			return
		}
		// Если стиль еще не отправлен этому получателю, отправляем его
		if !sentStyle[receiverID] {
			p2p.SendStyle(h, receiverInfo, styleFile)
			sentStyle[receiverID] = true
		}

		// Отправляем изображение
		fmt.Printf("📤 Отправляем %s ➜ %s...\n", file.Name(), receiverID)
		p2p.SendImage(h, receiverInfo, imagePath)
	}

	fmt.Println("✅ Все изображения отправлены. Ожидайте обработанные результаты.")
	select {}
}
