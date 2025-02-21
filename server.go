package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"sync"

	"github.com/markaya/foreverstore/p2p"
)

type FileServerOpts struct {
	StorageRoot       string
	PathTransformFunc PathTransformFunc
	Transport         p2p.Transport
	BootstrapNodes    []string
}

type FileServer struct {
	FileServerOpts

	peerLock sync.Mutex
	peers    map[string]p2p.Peer

	store  *Store
	quitch chan struct{}
}

func NewFileServer(opts FileServerOpts) *FileServer {
	storeOpts := StoreOpts{
		Root:              opts.StorageRoot,
		PathTransformFunc: opts.PathTransformFunc,
	}
	return &FileServer{
		FileServerOpts: opts,
		store:          NewStore(storeOpts),
		quitch:         make(chan struct{}),
		peers:          make(map[string]p2p.Peer),
	}
}

type Message struct {
	Payload any
}

type MessageStoreFile struct {
	Key string
}

func (s *FileServer) broadcast(msg *Message) error {
	peers := []io.Writer{}
	for _, peer := range s.peers {
		peers = append(peers, peer)
	}
	mw := io.MultiWriter(peers...)
	return gob.NewEncoder(mw).Encode(msg)
}

func (s *FileServer) StoreData(key string, r io.Reader) error {
	// buf := new(bytes.Buffer)
	// tee := io.TeeReader(r, buf)
	//
	// if err := s.store.Write(key, tee); err != nil {
	// 	return err
	// }
	//
	// _, err := io.Copy(buf, r)
	// if err != nil {
	// 	return err
	// }
	//
	// p := &DataMessage{
	// 	Key:  key,
	// 	Data: buf.Bytes(),
	// }
	//
	// return s.broadcast(&Message{
	// 	From:    "TODO",
	// 	Payload: p,
	// })

	buf := new(bytes.Buffer)
	msg := Message{
		Payload: MessageStoreFile{
			Key: key,
		},
	}

	if err := gob.NewEncoder(buf).Encode(msg); err != nil {
		return err
	}

	for _, peer := range s.peers {
		if err := peer.Send(buf.Bytes()); err != nil {
			return err
		}
	}

	// time.Sleep(time.Second * 3)
	// payload := []byte("This large file")
	//
	// for _, peer := range s.peers {
	// 	if err := peer.Send(payload); err != nil {
	// 		return err
	// 	}
	// }

	return nil
}

func (s *FileServer) Stop() {
	close(s.quitch)
}

func (s *FileServer) OnPeer(p p2p.Peer) error {
	s.peerLock.Lock()
	defer s.peerLock.Unlock()

	s.peers[p.RemoteAddr().String()] = p

	log.Printf("connected with remote %s", p.RemoteAddr())

	return nil
}

func (s *FileServer) loop() {
	defer func() {
		log.Println("file server stopped due user quit action")
		// TODO: What about error here?
		s.Transport.Close()
	}()
	for {
		select {
		case rpc := <-s.Transport.Consume():
			var msg Message
			if err := gob.NewDecoder(bytes.NewReader(rpc.Payload)).Decode(&msg); err != nil {
				log.Println(err)
			}
			fmt.Printf("Recv: %+vs\n", msg.Payload)

			peer, ok := s.peers[rpc.From]
			if !ok {
				panic("no peer in peers map")
			}

			b := make([]byte, 1000)
			if _, err := peer.Read(b); err != nil {
				panic(err)
			}

			fmt.Printf("big data: %s\n", string(b))

			// HACK: Not good
			peer.(*p2p.TCPPeer).Wg.Done()

			// if err := s.handleMessage(&m); err != nil {
			// 	log.Println(err)
			// }
		case <-s.quitch:
			return
		}
	}
}

// func (s *FileServer) handleMessage(msg *Message) error {
// 	switch v := msg.Payload.(type) {
// 	case *DataMessage:
// 		fmt.Printf("received data %+v \n", v)
// 	}
// 	return nil
// }

func (s *FileServer) boostrapNetwork() error {
	for _, addr := range s.BootstrapNodes {
		if len(addr) == 0 {
			continue
		}
		go func(addr string) {
			log.Println("[info] attempting to connect with remote: ", addr)
			if err := s.Transport.Dial(addr); err != nil {
				log.Println("[error] dial error: ", err)
			}
		}(addr)
	}

	return nil
}

func (s *FileServer) Start() error {
	if err := s.Transport.ListenAndAccept(); err != nil {
		return err
	}

	s.boostrapNetwork()
	s.loop()

	return nil
}

func init() {
	gob.Register(MessageStoreFile{})
}
