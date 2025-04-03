package connector

type Connector[T any] interface {
	Write(item T) error
	Read() (T, error)
	Close()
}
