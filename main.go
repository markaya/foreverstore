package main

import (
	"fmt"
	"log"

	"github.com/markaya/foreverstore/p2p"
)

func OnPeer(peer p2p.Peer) error {

	peer.Close()

	fmt.Println("Doing some logic with the peer outside of TCPTransport")
	return nil
	// return fmt.Errorf("failed the onpeer func")
}

func main() {
	tcpOpts := p2p.TCPTransportOpts{
		ListenAddr:    ":3000",
		HandshakeFunc: p2p.NOPHandshakeFunc,
		Decoder:       p2p.DefaultDecoder{},
		OnPeer:        OnPeer,
	}
	tr := p2p.NewTCPTransport(tcpOpts)

	go func() {
		for {
			msg := <-tr.Consume()
			fmt.Printf("%+v\n", msg)
		}
	}()

	if err := tr.ListenAndAccept(); err != nil {
		log.Fatal(err)
	}

	select {}

}
