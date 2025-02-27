package main

import (
	"bytes"
	"fmt"
	"io"
	"testing"
)

func TestPathTransformFunc(t *testing.T) {
	key := "momsbestpicture"
	pathkey := CASPathTransformFunc(key)

	expectedPathName := "68044/29f74/181a6/3c50c/3d81d/733a1/2f14a/353ff"
	expectedFilename := "6804429f74181a63c50c3d81d733a12f14a353ff"

	if pathkey.PathName != expectedPathName {
		t.Errorf("have %s, expected %s", pathkey.PathName, expectedPathName)
	}
	if pathkey.Filename != expectedFilename {
		t.Errorf("have %s, expected %s", pathkey.Filename, expectedFilename)
	}
}

func TestStore(t *testing.T) {
	s := newStore()
	defer teardown(t, s)

	for i := range 50 {
		key := fmt.Sprintf("fooobaaar_%d", i)
		data := []byte("some jpg bytes")

		_, err := s.writeStream(key, bytes.NewReader(data))
		if err != nil {
			t.Error(err)
		}
		if ok := s.Has(key); !ok {
			t.Errorf("expected to have key %s", key)
		}

		_, r, err := s.Read(key)
		if err != nil {
			t.Error(err)
		}

		b, _ := io.ReadAll(r)

		if string(b) != string(data) {
			t.Errorf("want %s, have %s", data, b)
		}

		if err := s.Delete(key); err != nil {
			t.Error(err)
		}

		if ok := s.Has(key); ok {
			t.Errorf("expected to have key %s", key)
		}
	}
}

func newStore() *Store {
	opts := StoreOpts{
		PathTransformFunc: CASPathTransformFunc,
	}

	return NewStore(opts)
}

func teardown(t *testing.T, s *Store) {
	if err := s.Clear(); err != nil {
		t.Error(err)
	}

}
