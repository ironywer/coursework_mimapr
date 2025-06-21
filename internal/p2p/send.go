package p2p

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	peerstore "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"

	host "github.com/libp2p/go-libp2p/core/host"
)

// Отправка файла стиля по протоколу "/receive-style/1.0.0"
func SendStyle(h host.Host, receiver peerstore.AddrInfo, stylePath string) {
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
func SendImage(h host.Host, receiver peerstore.AddrInfo, imagePath string) {
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

// Функция отправки обработанного изображения обратно отправителю (в режиме процессора)
func SendProcessedImage(h host.Host, receiver peerstore.ID, addrs []ma.Multiaddr, filePath string, failed bool, errMsg string) {
	receiverInfo := peerstore.AddrInfo{ID: receiver, Addrs: addrs}
	h.Peerstore().AddAddrs(receiverInfo.ID, receiverInfo.Addrs, time.Minute)

	if err := h.Connect(context.Background(), receiverInfo); err != nil {
		log.Println("❌ Ошибка подключения к получателю:", err)
		return
	}

	stream, err := h.NewStream(context.Background(), receiver, "/receive-image-result/1.0.0")
	if err != nil {
		log.Println("❌ Ошибка установления потока:", err)
		return
	}
	defer stream.Close()

	// Обработка ошибок передачи
	if failed || filePath == "" {
		_, _ = stream.Write([]byte("ERROR\n" + errMsg + "\n"))
		log.Println("⚠️ Отправлено сообщение об ошибке:", errMsg)
		return
	}

	// Проверка на существование файла
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		errMsg := fmt.Sprintf("Файл результата не найден: %s", filePath)
		_, _ = stream.Write([]byte("ERROR\n" + errMsg + "\n"))
		log.Println("⚠️ Ошибка: файл результата не существует")
		return
	}

	file, err := os.Open(filePath)
	if err != nil {
		log.Println("❌ Ошибка открытия файла результата:", err)
		errMsg := fmt.Sprintf("Не удалось открыть файл результата: %v", err)
		_, _ = stream.Write([]byte("ERROR\n" + errMsg + "\n"))
		return
	}
	defer file.Close()

	// Заголовок и передача данных
	_, _ = stream.Write([]byte("IMAGE\n"))
	if _, err := io.Copy(stream, file); err != nil {
		log.Println("❌ Ошибка отправки файла результата:", err)
	}
}
