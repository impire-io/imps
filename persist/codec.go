package persist

import "encoding/json"

// Codec encodes and decodes one state kind. The default is JSONCodec; a
// replacement's output is carried opaquely by the envelope, so any byte
// encoding works.
type Codec[T any] interface {
	Marshal(T) ([]byte, error)
	Unmarshal([]byte) (T, error)
}

// JSONCodec is the default codec (encoding/json).
func JSONCodec[T any]() Codec[T] { return jsonCodec[T]{} }

type jsonCodec[T any] struct{}

func (jsonCodec[T]) Marshal(v T) ([]byte, error) { return json.Marshal(v) }

func (jsonCodec[T]) Unmarshal(data []byte) (T, error) {
	var v T
	err := json.Unmarshal(data, &v)
	return v, err
}
