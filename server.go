package main

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/markaya/foreverstore/p2p"
)

type FileServerOpts struct {
	EncKey            []byte
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
	Key  string
	Size int64
}

type MessageGetFile struct {
	Key string
}

func (s *FileServer) broadcast(msg *Message) error {

	buf := new(bytes.Buffer)
	if err := gob.NewEncoder(buf).Encode(msg); err != nil {
		return err
	}

	for _, peer := range s.peers {
		peer.Send([]byte{p2p.IncomingMessage})
		time.Sleep(time.Millisecond * 10)
		if err := peer.Send(buf.Bytes()); err != nil {
			return err
		}
	}
	return nil
}

func (s *FileServer) Get(key string) (io.Reader, error) {
	if s.store.Has(key) {
		fmt.Printf(
			"[%s] serving file (%s) from local disk\n",
			s.Transport.Addr(),
			key,
		)
		_, r, err := s.store.Read(key)
		return r, err
	}

	fmt.Printf(
		"[%s] don't have file (%s) locally, fetching from network... \n",
		s.Transport.Addr(),
		key,
	)

	msg := Message{
		Payload: MessageGetFile{
			Key: key,
		},
	}

	if err := s.broadcast(&msg); err != nil {
		return nil, err
	}

	time.Sleep(time.Millisecond * 500)

	for _, peer := range s.peers {
		// First read the file size so we can limit the amount of bytes that we read
		// from the connection so it will not keep hanging.
		var fileSize int64
		err := binary.Read(peer, binary.LittleEndian, &fileSize)
		if err != nil {
			panic(err)
		}

		n, err := s.store.WriteDecrypt(s.EncKey, key, io.LimitReader(peer, fileSize))
		if err != nil {
			return nil, err
		}

		fmt.Printf("[%s] received (%d) bytes over the network from (%s): \n",
			s.Transport.Addr(),
			n,
			peer.RemoteAddr(),
		)
		peer.CloseStream()
	}

	_, r, err := s.store.Read(key)
	return r, err
}

func (s *FileServer) Store(key string, r io.Reader) error {

	var (
		fileBuffer = new(bytes.Buffer)
		tee        = io.TeeReader(r, fileBuffer)
	)

	size, err := s.store.Write(key, tee)
	if err != nil {
		return err
	}

	msg := Message{
		Payload: MessageStoreFile{
			Key:  key,
			Size: size + 16,
		},
	}

	if err := s.broadcast(&msg); err != nil {
		return err
	}

	//FIXME: Why this??
	time.Sleep(time.Millisecond * 5)

	peers := []io.Writer{}

	for _, peer := range s.peers {
		peers = append(peers, peer)
	}
	mw := io.MultiWriter(peers...)
	if _, err := mw.Write([]byte{p2p.IncomingStream}); err != nil {
		return err
	}
	n, err := copyEncrypt(s.EncKey, fileBuffer, mw)
	if err != nil {
		return err
	}

	fmt.Printf("[%s] received and wirtten (%d) bytes to disk\n", s.Transport.Addr(), n)

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
		log.Println("file server stopped due to error or user quit action")
		// TODO: What about error here?
		s.Transport.Close()
	}()
	for {
		select {
		case rpc := <-s.Transport.Consume():
			var msg Message
			if err := gob.NewDecoder(bytes.NewReader(rpc.Payload)).Decode(&msg); err != nil {
				log.Println("decoding er:", err)
			}

			if err := s.handleMessage(rpc.From, &msg); err != nil {
				log.Println("handle message error:", err)
				// continue
			}

		case <-s.quitch:
			return
		}
	}
}

func (s *FileServer) handleMessage(from string, msg *Message) error {
	switch v := msg.Payload.(type) {
	case MessageStoreFile:
		fmt.Printf("received data %+v \n", v)
		return s.handleMessageStoreFile(from, v)
	case MessageGetFile:
		return s.handleMessageGetFile(from, v)
	}
	return nil
}

func (s *FileServer) handleMessageGetFile(from string, msg MessageGetFile) error {
	if !s.store.Has(msg.Key) {
		return fmt.Errorf(
			"[%s] need to serve file (%s) but it does not exist on disk",
			s.Transport.Addr(),
			msg.Key,
		)
	}

	fmt.Printf(
		"[%s] serving file (%s) over the network...\n",
		s.Transport.Addr(),
		msg.Key,
	)

	fileSize, r, err := s.store.Read(msg.Key)
	if err != nil {
		return err
	}

	//WARNING: How to fix this not to reapeat readercloseer everytime?
	if rc, ok := r.(io.ReadCloser); ok {
		fmt.Println("closing ReadCloser")
		defer rc.Close()
	} else {
		panic("Here")
	}

	peer, ok := s.peers[from]
	if !ok {
		return fmt.Errorf("peer %s not in map\n", from)
	}

	// Fist send the "incomingstream" byte to peer.
	// Then we can send file size sa int64.
	err = peer.Send([]byte{p2p.IncomingStream})
	binary.Write(peer, binary.LittleEndian, fileSize)
	if err != nil {
		return err
	}

	n, err := io.Copy(peer, r)
	if err != nil {
		return err
	}

	fmt.Printf("[%s] written (%d) bytes over the network to %s\n",
		s.Transport.Addr(),
		n,
		from,
	)

	return nil
}

func (s *FileServer) handleMessageStoreFile(from string, msg MessageStoreFile) error {
	peer, ok := s.peers[from]

	if !ok {
		return fmt.Errorf("peer (%s) could not be found in the peer list", from)
	}
	n, err := s.store.Write(msg.Key, io.LimitReader(peer, msg.Size))
	if err != nil {
		fmt.Printf("Error Here: %s", err)
		return err
	}

	fmt.Printf("[%s] written %d bytes to disk \n", s.Transport.Addr(), n)

	peer.CloseStream()

	return nil
}

func (s *FileServer) boostrapNetwork() error {

	fmt.Println("Here", s.BootstrapNodes)
	for _, addr := range s.BootstrapNodes {
		if len(addr) == 0 {
			continue
		}
		go func(addr string) {
			log.Printf("[%s] attempting to connect with remote: %s\n ", s.Transport.Addr(), addr)
			if err := s.Transport.Dial(addr); err != nil {
				log.Println("[error] dial error: ", err)
			}
		}(addr)
	}

	return nil
}

func (s *FileServer) Start() error {
	fmt.Println("Starting file servver")
	if err := s.Transport.ListenAndAccept(); err != nil {
		return err
	}

	s.boostrapNetwork()
	s.loop()

	return nil
}

func init() {
	gob.Register(MessageStoreFile{})
	gob.Register(MessageGetFile{})
}
