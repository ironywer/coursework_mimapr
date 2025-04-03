package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	host "github.com/libp2p/go-libp2p/core/host"
	network "github.com/libp2p/go-libp2p/core/network"
	peerstore "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

var styleFile = "style.pt" // Файл, в котором будут признаки стиля

// sentStyle отслеживает, для каких пиров уже отправлены признаки стиля.
var sentStyle = make(map[peerstore.ID]bool)

// saveStreamToFile читает весь поток и сохраняет его в указанный файл.
func saveStreamToFile(s network.Stream, path string) error {
	// Создаем папку, если нужно
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, s)
	return err
}

// isImageFile проверяет расширение файла
func isImageFile(file fs.DirEntry) bool {
	name := strings.ToLower(file.Name())
	return strings.HasSuffix(name, ".jpg") ||
		strings.HasSuffix(name, ".jpeg") ||
		strings.HasSuffix(name, ".png")
}

func main() {
	// Определяем режим работы: "initiator" или "processor" (по умолчанию initiator)
	mode := "initiator"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	fmt.Println("Режим работы:", mode)

	// Создаем P2P-узел с открытым портом
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
		h.SetStreamHandler("/receive-style/1.0.0", handleReceiveStyle)
		h.SetStreamHandler("/receive-image/1.0.0", handleReceiveImage)
		fmt.Println("🔧 Режим процессора: обработчики для /receive-style/1.0.0 и /receive-image/1.0.0 зарегистрированы.")
		// Режим процессора работает только для обработки входящих данных
		select {}
	}

	// Режим инициатора:
	// 1. Извлекаем стиль
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("\n🖌 Введите путь к изображению-стилю: ")
	styleImgPath, _ := reader.ReadString('\n')
	styleImgPath = strings.TrimSpace(styleImgPath)

	fmt.Println("⏳ Извлечение признаков стиля...")
	cmd := exec.Command("python3", "style_transfer.py", "extract-style", styleImgPath, styleFile)
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
		if file.IsDir() || !isImageFile(file) {
			continue
		}
		imagePath := filepath.Join(dirPath, file.Name())

		receiverID, receiverAddrs := requestPeer(h, *bootstrapInfo)
		if receiverID == "" {
			fmt.Println("⚠️ Нет доступных получателей для файла:", file.Name())
			continue
		}
		receiverInfo := peerstore.AddrInfo{ID: receiverID, Addrs: receiverAddrs}
		h.Peerstore().AddAddrs(receiverInfo.ID, receiverInfo.Addrs, time.Hour)

		// Если стиль еще не отправлен этому получателю, отправляем его
		if !sentStyle[receiverID] {
			sendStyle(h, receiverInfo, styleFile)
			sentStyle[receiverID] = true
		}

		// Отправляем изображение
		fmt.Printf("📤 Отправляем %s ➜ %s...\n", file.Name(), receiverID)
		sendImage(h, receiverInfo, imagePath)
	}

	fmt.Println("✅ Все изображения отправлены. Ожидайте обработанные результаты.")
	select {}
}

// Запрос назначения пира у сервера
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
		log.Println("❌ Ошибка чтения ответа от сервера:", err)
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

// Отправка файла стиля по протоколу "/receive-style/1.0.0"
func sendStyle(h host.Host, receiver peerstore.AddrInfo, stylePath string) {
	file, err := os.Open(stylePath)
	if err != nil {
		log.Println("❌ Ошибка открытия файла стиля:", err)
		return
	}
	defer file.Close()
	stream, err := h.NewStream(context.Background(), receiver.ID, "/receive-style/1.0.0")
	if err != nil {
		log.Println("❌ Ошибка соединения для отправки стиля:", err)
		return
	}
	defer stream.Close()
	_, err = stream.Write([]byte("STYLE\n"))
	if err != nil {
		log.Println("❌ Ошибка отправки заголовка стиля:", err)
		return
	}
	_, err = io.Copy(stream, file)
	if err != nil {
		log.Println("❌ Ошибка отправки файла стиля:", err)
		return
	}
	fmt.Println("✅ Признаки стиля отправлены получателю:", receiver.ID)
}

// Отправка изображения по протоколу "/receive-image/1.0.0"
func sendImage(h host.Host, receiver peerstore.AddrInfo, imagePath string) {
	file, err := os.Open(imagePath)
	if err != nil {
		log.Println("❌ Ошибка открытия файла изображения:", err)
		return
	}
	defer file.Close()
	stream, err := h.NewStream(context.Background(), receiver.ID, "/receive-image/1.0.0")
	if err != nil {
		log.Println("❌ Ошибка соединения для отправки изображения:", err)
		return
	}
	defer stream.Close()
	_, err = stream.Write([]byte("IMAGE\n"))
	if err != nil {
		log.Println("❌ Ошибка отправки заголовка изображения:", err)
		return
	}
	_, err = io.Copy(stream, file)
	if err != nil {
		log.Println("❌ Ошибка отправки изображения:", err)
		return
	}
	fmt.Println("✅ Изображение успешно отправлено:", filepath.Base(imagePath))
}

// Обработчик получения обработанных изображений по протоколу "/receive-image-result/1.0.0"
func receiveProcessedImage(s network.Stream) {
	defer s.Close()
	peerID := s.Conn().RemotePeer().String()
	timestamp := time.Now().UnixNano()
	// Сохраняем в папку processed_images
	dir := "processed_images"
	os.MkdirAll(dir, 0755)
	fileName := fmt.Sprintf("%s/processed_%s_%d.jpg", dir, peerID, timestamp)
	file, err := os.Create(fileName)
	if err != nil {
		log.Println("❌ Ошибка создания файла результата:", err)
		return
	}
	defer file.Close()
	_, err = io.Copy(file, s)
	if err != nil {
		log.Println("❌ Ошибка записи обработанного изображения:", err)
		return
	}
	fmt.Println("✅ Обработанное изображение сохранено как:", fileName)
}

// ================= Режим процессора =================


// Обработчик получения файла стиля по протоколу "/receive-style/1.0.0"
func handleReceiveStyle(s network.Stream) {
	defer s.Close()
	reader := bufio.NewReader(s)

	// Читаем заголовок
	header, err := reader.ReadString('\n')
	if err != nil {
		log.Println("❌ Ошибка чтения заголовка стиля:", err)
		return
	}
	header = strings.TrimSpace(header)
	if header != "STYLE" {
		log.Println("❌ Ожидался заголовок 'STYLE', получено:", header)
		return
	}

	// Сохраняем оставшиеся данные в файл
	dir := "received_styles"
	os.MkdirAll(dir, 0755)
	fileName := fmt.Sprintf("%s/received_style_%d.pt", dir, time.Now().UnixNano())
	err = saveReaderToFile(reader, fileName)
	if err != nil {
		log.Println("❌ Ошибка сохранения файла стиля:", err)
		return
	}
	fmt.Println("🎨 Файл стиля получен и сохранен как:", fileName)
	// Обновляем локальный styleFile для обработки
	styleFile = fileName
}

// Обработчик получения изображения для стилизации по протоколу "/receive-image/1.0.0"
func handleReceiveImage(s network.Stream) {
	defer s.Close()
	// Оборачиваем поток в bufio.Reader
	reader := bufio.NewReader(s)
	// Читаем заголовок (ожидается "IMAGE")
	header, err := reader.ReadString('\n')
	if err != nil {
		log.Println("❌ Ошибка чтения заголовка:", err)
		return
	}
	header = strings.TrimSpace(header)
	if header != "IMAGE" {
		log.Println("❌ Неверный заголовок, ожидается IMAGE, получено:", header)
		return
	}

	// Сохраняем оставшиеся данные в файл, создавая уникальное имя в папке "received_images"
	dir := "received_images"
	os.MkdirAll(dir, 0755)
	tmpIn := fmt.Sprintf("%s/received_%d.jpg", dir, time.Now().UnixNano())
	err = saveReaderToFile(reader, tmpIn)
	if err != nil {
		log.Println("❌ Ошибка сохранения полученного изображения:", err)
		return
	}
	fmt.Println("📥 Изображение получено:", tmpIn)

	// Запускаем стилизацию с использованием локального styleFile
	dirOut := "processed_images"
	os.MkdirAll(dirOut, 0755)
	tmpOut := fmt.Sprintf("%s/styled_%d.jpg", dirOut, time.Now().UnixNano())
	cmd := exec.Command("python3", "style_transfer.py", "stylize", tmpIn, styleFile, tmpOut)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Println("⏳ Запуск стилизации для", tmpIn)
	if err := cmd.Run(); err != nil {
		log.Println("❌ Ошибка стилизации:", err)
		return
	}
	fmt.Println("🖼 Стилизация завершена:", tmpOut)
	// Отправляем результат обратно по протоколу "/receive-image-result/1.0.0"
	sendProcessedImage(s.Conn().RemotePeer(), tmpOut)
	// Удаляем временные файлы
	os.Remove(tmpIn)
	os.Remove(tmpOut)
}

// Вспомогательная функция для сохранения данных из bufio.Reader в файл
func saveReaderToFile(r *bufio.Reader, path string) error {
	dir := filepath.Dir(path)
	os.MkdirAll(dir, 0755)
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, r)
	return err
}

// Функция отправки обработанного изображения обратно отправителю (в режиме процессора)
func sendProcessedImage(receiver peerstore.ID, filePath string) {
	// Создаем новый хост для отправки результата
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"))
	if err != nil {
		log.Println("❌ Ошибка создания хоста для отправки результата:", err)
		return
	}
	defer h.Close()
	stream, err := h.NewStream(context.Background(), receiver, "/receive-image-result/1.0.0")
	if err != nil {
		log.Println("❌ Ошибка установки соединения для отправки результата:", err)
		return
	}
	defer stream.Close()
	file, err := os.Open(filePath)
	if err != nil {
		log.Println("❌ Ошибка открытия обработанного файла:", err)
		return
	}
	defer file.Close()
	_, err = io.Copy(stream, file)
	if err != nil {
		log.Println("❌ Ошибка отправки обработанного изображения:", err)
		return
	}
	fmt.Println("✅ Обработанный файл отправлен обратно:", filePath)
}
