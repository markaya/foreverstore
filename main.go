package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/markaya/foreverstore/p2p"
)

// TODO: Remove
var _ = io.ReadAll
var _ = fmt.Print
var _ = bytes.Join

func makeServer(listenAddr string, nodes ...string) *FileServer {
	tcptransportOpts := p2p.TCPTransportOpts{
		ListenAddr:    listenAddr,
		HandshakeFunc: p2p.NOPHandshakeFunc,
		Decoder:       p2p.DefaultDecoder{},
		//  TODO: onPeer Func
	}

	tcpTransport := p2p.NewTCPTransport(tcptransportOpts)

	fileServerOpts := FileServerOpts{
		EncKey:            newEncryptionKey(),
		StorageRoot:       listenAddr + "_network",
		PathTransformFunc: CASPathTransformFunc,
		Transport:         tcpTransport,
		BootstrapNodes:    nodes,
	}

	s := NewFileServer(fileServerOpts)

	tcpTransport.OnPeer = s.OnPeer

	return s
}

func main() {

	s1 := makeServer(":4000", "")
	s2 := makeServer(":5001", "")
	s3 := makeServer(":3000", ":4000", ":5001")

	go func() {
		log.Fatal(s1.Start())
	}()

	time.Sleep(500 * time.Millisecond)
	go func() {
		log.Fatal(s2.Start())
	}()

	time.Sleep(2 * time.Second)
	go s3.Start()
	time.Sleep(5 * time.Second)

	for i := range 2 {
		key := fmt.Sprintf("picture_%d.png", i)
		data := bytes.NewReader([]byte("my big data file here!"))
		s3.Store(key, data)

		if err := s3.store.Delete(key); err != nil {
			log.Fatal(err)
		}

		r, err := s3.Get(key)
		if err != nil {
			log.Fatal(err)
		}

		b, err := io.ReadAll(r)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Println(string(b))
	}

}
