package main

import (
	"bytes"
	"io"
	"testing"
)

func TestPathTransformFunc(t *testing.T) {
	key := "momsbestpicture"
	pathkey := CASPathTransformFunc(key)

	expectedPathName := "68044/29f74/181a6/3c50c/3d81d/733a1/2f14a/353ff"
	expectedOriginalKey := "6804429f74181a63c50c3d81d733a12f14a353ff"
	if pathkey.PathName != expectedPathName {
		t.Errorf("have %s, expected %s", pathkey.PathName, expectedPathName)
	}
	if pathkey.Filename != expectedOriginalKey {
		t.Errorf("have %s, expected %s", pathkey.Filename, expectedOriginalKey)
	}
}

func TestStore(t *testing.T) {
	opts := StoreOpts{
		PathTransformFunc: CASPathTransformFunc,
	}
	s := NewStore(opts)

	key := "momsspecieals"
	data := []byte("some jpg bytes")

	if err := s.writeStream(key, bytes.NewReader(data)); err != nil {
		t.Error(err)
	}
	if ok := s.Has(key); !ok {
		t.Errorf("expected to have key %s", key)
	}

	r, err := s.Read(key)
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

}
