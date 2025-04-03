package main

import (
	"fmt"
	"log"

	libp2p "github.com/libp2p/go-libp2p"
)

func main() {
	// Создаём P2P-узел
	host, err := libp2p.New()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("P2P-узел запущен!")
	fmt.Println("ID узла:", host.ID())

	// Держим процесс активным
	select {}
}
