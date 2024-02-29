package debouncer

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"sync"
)

type GobSerializer[V any] struct {
	pool sync.Pool
}

func NewGobSerializer[V any]() *GobSerializer[V] {
	var v V
	gob.Register(v)
	return &GobSerializer[V]{
		pool: sync.Pool{
			New: func() any { return &bytes.Buffer{} },
		},
	}
}

func (s *GobSerializer[V]) Serialize(value V) ([]byte, error) {
	buff, _ := s.pool.Get().(*bytes.Buffer)
	defer func() {
		buff.Reset()
		s.pool.Put(buff)
	}()
	if err := gob.NewEncoder(buff).Encode(value); err != nil {
		return nil, err
	}
	return buff.Bytes(), nil
}

func (s *GobSerializer[V]) Deserilize(data []byte) (V, error) {
	buff, _ := s.pool.Get().(*bytes.Buffer)
	_, _ = buff.Write(data)
	defer func() {
		buff.Reset()
		s.pool.Put(buff)
	}()
	var item V
	if err := gob.NewDecoder(buff).Decode(&item); err != nil {
		return item, err
	}
	return item, nil
}

type JSONSerializer[V any] struct{}

func (JSONSerializer[V]) Serialize(value V) ([]byte, error) {
	return json.Marshal(value)
}

func (JSONSerializer[V]) Deserilize(data []byte) (V, error) {
	var v V
	err := json.Unmarshal(data, &v)
	return v, err
}
