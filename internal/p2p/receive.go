package p2p

import (
	"bufio"
	"coursework_mimapr/internal/style"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	network "github.com/libp2p/go-libp2p/core/network"
	ma "github.com/multiformats/go-multiaddr"
)

var styleFile = "style.pt" // Файл, в котором будут признаки стиля

// Обработчик получения обработанных изображений по протоколу "/receive-image-result/1.0.0"
func ReceiveProcessedImage(s network.Stream) {
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
func HandleReceiveStyle(s network.Stream) {
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
	err = SaveReaderToFile(reader, fileName)
	if err != nil {
		log.Println("❌ Ошибка сохранения файла стиля:", err)
		return
	}
	fmt.Println("🎨 Файл стиля получен и сохранен как:", fileName)
	// Обновляем локальный styleFile для обработки
	styleFile = fileName
}

// Обработчик получения изображения для стилизации по протоколу "/receive-image/1.0.0"
func HandleReceiveImage(s network.Stream) {
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
	err = SaveReaderToFile(reader, tmpIn)
	if err != nil {
		log.Println("❌ Ошибка сохранения полученного изображения:", err)
		return
	}
	fmt.Println("📥 Изображение получено:", tmpIn)

	// Запускаем стилизацию с использованием локального styleFile
	dirOut := "processed_images"
	os.MkdirAll(dirOut, 0755)
	tmpOut := fmt.Sprintf("%s/styled_%d.jpg", dirOut, time.Now().UnixNano())
	// Сохраняем путь к адресам
	addrs := []ma.Multiaddr{s.Conn().RemoteMultiaddr()}

	cmd := exec.Command(style.GetPythonCommand(), "style_transfer.py", "stylize", tmpIn, styleFile, tmpOut)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Println("⏳ Запуск стилизации для", tmpIn)
	if err := cmd.Run(); err != nil {
		log.Println("❌ Ошибка стилизации:", err)
		SendProcessedImage(s.Conn().RemotePeer(), addrs, "", true, "Ошибка стилизации изображения")
		os.Remove(tmpIn)
		return
	}
	fmt.Println("🖼 Стилизация завершена:", tmpOut)

	// Отправляем результат
	SendProcessedImage(s.Conn().RemotePeer(), addrs, tmpOut, false, "")

	// Удаляем временные файлы
	os.Remove(tmpIn)
	os.Remove(tmpOut)
}

// SaveStreamToFile читает весь поток и сохраняет его в указанный файл.
func SaveStreamToFile(s network.Stream, path string) error {
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

// Вспомогательная функция для сохранения данных из bufio.Reader в файл
func SaveReaderToFile(r *bufio.Reader, path string) error {
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
